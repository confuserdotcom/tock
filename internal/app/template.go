package app

import (
	"errors"
	"fmt"
	"strings"

	"tock/internal/classify"
	"tock/internal/config"
	"tock/internal/rulesfile"
	"tock/internal/span"
)

// TemplateGroup is the capability group a template wants on, as config.toml
// has it now. Templates never write config.toml: a group is on unless you
// turned it off, and one you turned off stays off (On false, with the
// command that turns it on).
type TemplateGroup struct {
	Template, Group string
	On              bool
}

// TemplatePlan is classify.TemplatePlan with the groups and, for a plan
// that applies to the past, its effect on the last StatsDays days.
type TemplatePlan struct {
	classify.TemplatePlan
	Names  []string
	Past   bool
	Groups []TemplateGroup
	// With Past: time the new rules would label that is unlabelled now,
	// and time the local model labelled that they would label otherwise.
	PastNewMS, PastChangedMS int64
	Days                     int
}

// PlanTemplates plans applying the named templates to rules.toml. Without
// past their rules start now (since), so no label before now changes.
func (a *App) PlanTemplates(names []string, past bool) (TemplatePlan, error) {
	p, cur, err := planTemplates(a.rules(), a.cfg, names, a.nowMS(), past)
	if err != nil {
		return TemplatePlan{}, err
	}
	cp := p.TemplatePlan
	if past && len(cp.AddRules) > 0 {
		cand, err := classify.Parse(cp.Text, cur.Learned)
		if err != nil {
			return TemplatePlan{}, err
		}
		if p.PastNewMS, p.PastChangedMS, err = a.pastEffect(cand); err != nil {
			return TemplatePlan{}, err
		}
	}
	return p, nil
}

// planTemplates plans from the rule files as they are now; cfg says which
// groups are on. It needs no database (see ApplyTemplatesNoDB).
func planTemplates(f rulesfile.Files, cfg config.Config, names []string, nowMS int64, past bool) (TemplatePlan, rulesfile.Set, error) {
	tpls, err := classify.TemplatesByName(names)
	if err != nil {
		return TemplatePlan{}, rulesfile.Set{}, err
	}
	if len(tpls) == 0 {
		return TemplatePlan{}, rulesfile.Set{}, errors.New("template: name one: " + strings.Join(classify.TemplateNames(), ", "))
	}
	cur, err := f.Read()
	if err != nil {
		return TemplatePlan{}, rulesfile.Set{}, err
	}
	rs, err := classify.Parse(cur.Rules, cur.Learned) // the file as it is now
	if err != nil {
		return TemplatePlan{}, rulesfile.Set{}, err
	}
	cp, err := classify.PlanTemplates(rs, cur.Rules, tpls, nowMS, past)
	if err != nil {
		return TemplatePlan{}, rulesfile.Set{}, err
	}
	p := TemplatePlan{TemplatePlan: cp, Past: past, Days: StatsDays}
	for _, t := range tpls {
		p.Names = append(p.Names, t.Name)
		on := true
		for _, ft := range config.Features {
			if ft.ID == t.Group {
				on = ft.Set(cfg)
			}
		}
		p.Groups = append(p.Groups, TemplateGroup{Template: t.Title, Group: t.Group, On: on})
	}
	return p, cur, nil
}

// PlanTemplatesNoDB is PlanTemplates before tock has a database: rules
// start at nowMS and there is no past to look at.
func PlanTemplatesNoDB(f rulesfile.Files, cfg config.Config, names []string, nowMS int64) (TemplatePlan, error) {
	p, _, err := planTemplates(f, cfg, names, nowMS, false)
	return p, err
}

// ApplyTemplatesNoDB applies templates before tock has a database (a fresh
// setup): their rules start at nowMS, and there is no undo entry, since
// there is no history to protect or journal to keep it in.
func ApplyTemplatesNoDB(f rulesfile.Files, cfg config.Config, names []string, nowMS int64) (TemplatePlan, error) {
	var p TemplatePlan
	err := f.Lock(func() error {
		var (
			cur rulesfile.Set
			err error
		)
		if p, cur, err = planTemplates(f, cfg, names, nowMS, false); err != nil || p.Empty() {
			return err
		}
		next := cur
		next.Rules = p.Text
		return f.Write(cur, next, rulesfile.Options{})
	})
	return p, err
}

// pastEffect labels the last StatsDays days with the current rules and with
// cand, the local model included, and returns the time cand newly labels
// and the time it takes from the model.
func (a *App) pastEffect(cand *classify.Ruleset) (newMS, changedMS int64, err error) {
	now := a.nowMS()
	from := now - int64(StatsDays)*libDayMS
	cut := a.cut
	defer func() { a.cut = cut }()
	rows, ovs, err := a.spans(from, now)
	if err != nil {
		return 0, 0, err
	}
	vs, err := a.decorate(a.base(rows, ovs, from, now, now), ovs, from, now)
	if err != nil {
		return 0, 0, err
	}
	m, err := a.labelModel()
	if err != nil {
		return 0, 0, err
	}
	before, after := a.rs.LabelAll(vs, ovs), cand.LabelAll(vs, ovs)
	a.rs.ApplyModel(m, vs, before)
	cand.ApplyModel(m, vs, after)
	for i := range vs {
		if vs[i].BundleID == span.IdleBundleID || after[i].Category == before[i].Category {
			continue
		}
		switch {
		case before[i].Category == "":
			newMS += vs[i].DurMS()
		case before[i].CategorySource == classify.SourceModel:
			changedMS += vs[i].DurMS()
		}
	}
	return newMS, changedMS, nil
}

// ApplyTemplates applies the named templates as PlanTemplates plans them,
// planned again under the rules lock so a change made since is not lost.
// It is one tock undo entry; an empty plan writes nothing.
func (a *App) ApplyTemplates(names []string, past bool) (TemplatePlan, error) {
	var p TemplatePlan
	err := a.withRulesLock(func() error {
		var err error
		if p, err = a.PlanTemplates(names, past); err != nil || p.Empty() {
			return err
		}
		f := a.rules()
		cur, err := f.Read()
		if err != nil {
			return err
		}
		next := cur
		next.Rules = p.Text
		if err := f.Write(cur, next, rulesfile.Options{}); err != nil {
			return err
		}
		if _, err := a.Reload(); err != nil {
			return err
		}
		return a.journal(fmt.Sprintf("template %s", strings.Join(p.Names, "+")),
			undoData{File: &undoFile{Path: a.paths.Rules, Existed: true, Before: string(cur.Rules), After: string(p.Text)}})
	})
	return p, err
}
