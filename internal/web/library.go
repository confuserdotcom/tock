package web

import (
	"errors"
	"fmt"
	"net/http"
	"syscall"

	"tock/internal/app"
	"tock/internal/classify"
)

type libCategoryJSON struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	AFKMS      int64  `json:"afkMS"`
	Interrupt  bool   `json:"interrupt"`
	Projects   int    `json:"projects"`
	Last30MS   int64  `json:"last30MS"`
	LastSeenMS int64  `json:"lastSeenMS"`
}

type libProjectJSON struct {
	Name       string   `json:"name"`
	Category   string   `json:"category"`
	Aliases    []string `json:"aliases"`
	Rate       *float64 `json:"rate,omitempty"` // left out while Billing is off
	Client     string   `json:"client,omitempty"`
	Archived   bool     `json:"archived"`
	Last30MS   int64    `json:"last30MS"`
	LastSeenMS int64    `json:"lastSeenMS"`
}

type libGoalJSON struct {
	Category string `json:"category"`
	Project  string `json:"project"`
	Name     string `json:"name,omitempty"`
	Text     string `json:"text,omitempty"`
	AtMost   bool   `json:"atMost"`
	Week     bool   `json:"week"`
	MS       int64  `json:"ms"`
}

type libDupJSON struct {
	Kind string `json:"kind"`
	A    string `json:"a"`
	B    string `json:"b"`
}

type libChangeJSON struct {
	What string `json:"what"`
	Undo bool   `json:"undo"`
}

// libErr turns an app error from a Library write into the HTTP answer:
// a busy rules lock is a conflict to retry, anything else the user's input.
func libErr(err error) error {
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return httpError{http.StatusConflict, "another tock is changing the rules: try again"}
	}
	return badRequest(err.Error())
}

// library lists every declared name with its recent use.
func (s *Server) library(r *http.Request) (any, error) {
	lib, err := s.a.Library()
	if err != nil {
		return nil, err
	}
	out := struct {
		Categories []libCategoryJSON `json:"categories"`
		Projects   []libProjectJSON  `json:"projects"`
		Goals      []libGoalJSON     `json:"goals"`
		Lookalikes []libDupJSON      `json:"lookalikes"`
		Currency   string            `json:"currency"`
		Billing    bool              `json:"billing"`
		Clients    []string          `json:"clients"` // [[invoice.client]] names
		NowMS      int64             `json:"nowMS"`
	}{Categories: []libCategoryJSON{}, Projects: []libProjectJSON{}, Goals: []libGoalJSON{}, Lookalikes: []libDupJSON{},
		Currency: s.a.Config().Invoice.Currency, Billing: s.a.Config().Billing.Enabled, Clients: []string{}, NowMS: s.now().UnixMilli()}
	for _, c := range s.a.Config().Invoice.Clients {
		out.Clients = append(out.Clients, c.Name)
	}
	for _, c := range lib.Categories {
		out.Categories = append(out.Categories, libCategoryJSON{c.Name, c.Kind, c.AFKMS, !c.Quiet, c.Projects, c.Last30MS, c.LastSeenMS})
	}
	for _, p := range lib.Projects {
		al := p.Aliases
		if al == nil {
			al = []string{}
		}
		rate := p.Rate
		if !out.Billing {
			rate = nil
		}
		out.Projects = append(out.Projects, libProjectJSON{p.Name, p.Category, al, rate, p.Client, p.Archived, p.Last30MS, p.LastSeenMS})
	}
	for _, g := range lib.Goals {
		out.Goals = append(out.Goals, libGoalJSON{g.Category, g.Project, g.Name(), g.Text(), g.AtMost, g.Week, g.MS})
	}
	for _, d := range lib.Lookalikes {
		out.Lookalikes = append(out.Lookalikes, libDupJSON{d.Kind, d.A, d.B})
	}
	return out, nil
}

// undoable runs fn under the rules undo and answers what it did.
func (s *Server) undoable(what string, fn func() error) (any, error) {
	if err := s.a.WithRulesUndo(what, fn); err != nil {
		return nil, libErr(err)
	}
	return libChangeJSON{what, true}, nil
}

// rename and remove share the category and project routes.
func (s *Server) renameOrRemove(kind, op, name, to string) (any, error) {
	switch op {
	case "rename":
		from, err := s.a.RenameName(kind, name, to)
		if err != nil {
			return nil, libErr(err)
		}
		return libChangeJSON{fmt.Sprintf("renamed %s %s to %s", kind, from, to), false}, nil
	case "remove":
		return s.undoable(fmt.Sprintf("removed %s %s", kind, name), func() error {
			_, err := s.a.RemoveName(kind, name)
			return err
		})
	}
	return nil, badRequest("op must be add, set, rename or remove")
}

func (s *Server) libCategory(r *http.Request) (any, error) {
	var b struct {
		Op        string `json:"op"`
		Name      string `json:"name"`
		New       string `json:"new"`
		Kind      string `json:"kind"`
		AFKMS     int64  `json:"afkMS"`
		Interrupt *bool  `json:"interrupt"`
	}
	if err := decode(r, &b); err != nil {
		return nil, err
	}
	spec := app.CategorySpec{Name: b.Name, Kind: b.Kind, AFKMS: b.AFKMS, Interrupt: b.Interrupt}
	switch b.Op {
	case "add":
		return s.undoable("added category "+b.Name, func() error {
			if err := s.a.AddCategory(spec); err != nil {
				return err
			}
			if b.Interrupt != nil && !*b.Interrupt {
				return s.a.EditCategory(app.CategorySpec{Name: b.Name, Interrupt: b.Interrupt})
			}
			return nil
		})
	case "set":
		return s.undoable("changed category "+b.Name, func() error { return s.a.EditCategory(spec) })
	}
	return s.renameOrRemove(classify.NameCategory, b.Op, b.Name, b.New)
}

func (s *Server) libProject(r *http.Request) (any, error) {
	var b struct {
		Op       string    `json:"op"`
		Name     string    `json:"name"`
		New      string    `json:"new"`
		Category *string   `json:"category"`
		Aliases  *[]string `json:"aliases"`
		Rate     *float64  `json:"rate"`
		Client   *string   `json:"client"`
		Archived *bool     `json:"archived"`
	}
	if err := decode(r, &b); err != nil {
		return nil, err
	}
	if !s.a.Config().Billing.Enabled {
		b.Rate, b.Client = nil, nil // Billing is off: they are not shown, so not edited
	}
	spec := app.ProjectSpec{Name: b.Name, Category: b.Category, Aliases: b.Aliases, Rate: b.Rate, Client: b.Client, Archived: b.Archived}
	switch b.Op {
	case "add":
		return s.undoable("added project "+b.Name, func() error {
			if err := s.a.AddProject(spec); err != nil {
				return err
			}
			if b.Archived != nil && *b.Archived {
				return s.a.EditProject(app.ProjectSpec{Name: b.Name, Archived: b.Archived})
			}
			return nil
		})
	case "set":
		return s.undoable("changed project "+b.Name, func() error { return s.a.EditProject(spec) })
	}
	return s.renameOrRemove(classify.NameProject, b.Op, b.Name, b.New)
}

func (s *Server) libGoal(r *http.Request) (any, error) {
	var b struct {
		Op   string       `json:"op"`
		Goal libGoalJSON  `json:"goal"`
		Was  *libGoalJSON `json:"was"`
	}
	if err := decode(r, &b); err != nil {
		return nil, err
	}
	g := classify.Goal{Category: b.Goal.Category, Project: b.Goal.Project, AtMost: b.Goal.AtMost, Week: b.Goal.Week, MS: b.Goal.MS}
	switch b.Op {
	case "set":
		return s.undoable("set goal "+g.Name(), func() error {
			if b.Was != nil {
				was := classify.Goal{Category: b.Was.Category, Project: b.Was.Project, AtMost: b.Was.AtMost, Week: b.Was.Week}
				if !classify.SameGoal(was, g) {
					if err := s.a.SetGoal(was); err != nil { // MS 0 removes it
						return err
					}
				}
			}
			return s.a.SetGoal(g)
		})
	case "remove":
		g.MS = 0
		return s.undoable("removed goal "+g.Name(), func() error { return s.a.SetGoal(g) })
	}
	return nil, badRequest("op must be set or remove")
}

func (s *Server) libMerge(r *http.Request) (any, error) {
	var b struct {
		Kind string `json:"kind"`
		From string `json:"from"`
		Into string `json:"into"`
	}
	if err := decode(r, &b); err != nil {
		return nil, err
	}
	if b.Kind != classify.NameCategory && b.Kind != classify.NameProject {
		return nil, badRequest("kind must be category or project")
	}
	src, dst, err := s.a.MergeNames(b.Kind, b.From, b.Into)
	if err != nil {
		return nil, libErr(err)
	}
	return libChangeJSON{fmt.Sprintf("merged %s %s into %s", b.Kind, src, dst), false}, nil
}
