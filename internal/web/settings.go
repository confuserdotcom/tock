package web

import (
	"fmt"
	"net/http"
	"strings"

	"tock/internal/config"
)

// featureJSON is one toggle of tock choice. On is config.toml's value; Live
// is whether it is in effect, which a member is not while its group is off.
type featureJSON struct {
	ID      string `json:"id"`
	Group   string `json:"group"`
	Name    string `json:"name"`
	Note    string `json:"note"`
	On      bool   `json:"on"`
	Live    bool   `json:"live"`
	IsGroup bool   `json:"isGroup"`
	Needs   string `json:"needs,omitempty"`
	Waits   bool   `json:"waits"`
}

type workHoursJSON struct {
	Set   bool     `json:"set"`
	Start string   `json:"start"` // "HH:MM"
	End   string   `json:"end"`
	Days  []string `json:"days"` // mon … sun
}

type clientJSON struct {
	Name    string   `json:"name"`
	Address []string `json:"address"`
	TaxID   string   `json:"taxId"`
}

// invoiceJSON is [invoice] as the page edits it; "" is not set.
type invoiceJSON struct {
	Name       string       `json:"name"`
	Address    []string     `json:"address"`
	Currency   string       `json:"currency"`
	Note       string       `json:"note"`
	TaxID      string       `json:"taxId"`
	Round      string       `json:"round"` // "15m"; "" = no rounding
	RoundMode  string       `json:"roundMode"`
	MinorUnits int          `json:"minorUnits"`
	TaxLabel   string       `json:"taxLabel"`
	TaxRate    string       `json:"taxRate"` // percent; "" = no tax line
	TaxNote    string       `json:"taxNote"`
	Clients    []clientJSON `json:"clients"`
}

type settingsJSON struct {
	Hours       workHoursJSON     `json:"hours"`
	Presets     []int         `json:"presets"`
	MaxSession  int           `json:"maxSession"`
	Invoice     invoiceJSON   `json:"invoice"`
	Features    []featureJSON `json:"features"`
	HideApps    []string      `json:"hideApps"`
	HideSites   []string      `json:"hideSites"`
	ConfigPath  string        `json:"configPath"`
	ConfigError string        `json:"configError,omitempty"`
}

func (s *Server) settings(*http.Request) (any, error) {
	cfg := s.a.Config()
	raw := cfg.Raw()
	out := settingsJSON{
		Hours:      hoursOf(raw),
		Presets:    append([]int{}, raw.Focus.Presets...),
		MaxSession: config.MaxSessionMin,
		Invoice:    invoiceOf(raw.Invoice),
		HideApps:   append([]string{}, cfg.Privacy.HideApps...),
		HideSites:  append([]string{}, cfg.Privacy.HideSites...),
		ConfigPath: s.a.ConfigPath(),
	}
	if err := s.a.ConfigErr(); err != nil {
		out.ConfigError = err.Error()
	}
	for _, f := range config.Features {
		out.Features = append(out.Features, featureJSON{
			ID: f.ID, Group: f.Group, Name: f.Name, Note: f.Note,
			On: f.Set(cfg), Live: f.On(cfg), IsGroup: f.IsGroup, Needs: f.Needs, Waits: f.Waits(cfg),
		})
	}
	return out, nil
}

// settingsSave writes feature changes to config.toml, as tock choice does,
// and answers with the settings as they now are.
func (s *Server) settingsSave(r *http.Request) (any, error) {
	var b struct {
		Features map[string]bool `json:"features"`
	}
	if err := decode(r, &b); err != nil {
		return nil, err
	}
	for id := range b.Features {
		if config.FeatureByID(id) == nil {
			return nil, badRequest(fmt.Sprintf("no setting %q", id))
		}
	}
	if err := config.SaveFeatures(s.a.ConfigPath(), b.Features); err != nil {
		return nil, badRequest(err.Error())
	}
	if _, err := s.a.Reload(); err != nil {
		return nil, err
	}
	return s.settings(r)
}

// settingsHide adds an app or a site to the [privacy] hide lists, or
// takes it off.
func (s *Server) settingsHide(r *http.Request) (any, error) {
	var b struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
		Hide  bool   `json:"hide"`
	}
	if err := decode(r, &b); err != nil {
		return nil, err
	}
	if b.Kind != config.HideApp && b.Kind != config.HideSite {
		return nil, badRequest("kind must be app or site")
	}
	if b.Value == "" {
		return nil, badRequest("value is empty")
	}
	if err := config.SetHidden(s.a.ConfigPath(), b.Kind, b.Value, b.Hide); err != nil {
		return nil, badRequest(err.Error())
	}
	if _, err := s.a.Reload(); err != nil {
		return nil, err
	}
	return s.settings(r)
}

var weekDays = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

func clockText(min int) string { return fmt.Sprintf("%02d:%02d", min/60, min%60) }

// hoursOf is [hours]; unset, it offers 09:00–17:00 on weekdays to start from.
func hoursOf(c config.Config) workHoursJSON {
	h := c.Hours
	if !h.Set {
		return workHoursJSON{Start: "09:00", End: "17:00", Days: weekDays[:5]}
	}
	out := workHoursJSON{Set: true, Start: clockText(h.StartMin), End: clockText(h.EndMin), Days: []string{}}
	for i, d := range weekDays {
		if h.Days[(i+1)%7] { // Days is indexed by time.Weekday: Sunday first
			out.Days = append(out.Days, d)
		}
	}
	return out
}

func roundText(s int64) string {
	switch {
	case s <= 1:
		return ""
	case s%3600 == 0:
		return fmt.Sprintf("%dh", s/3600)
	case s%60 == 0:
		return fmt.Sprintf("%dm", s/60)
	}
	return fmt.Sprintf("%ds", s)
}

func invoiceOf(i config.Invoice) invoiceJSON {
	out := invoiceJSON{
		Name: i.Name, Address: append([]string{}, i.Address...), Currency: i.Currency, Note: i.Note, TaxID: i.TaxID,
		Round: roundText(i.RoundStepS), RoundMode: i.RoundMode, MinorUnits: i.MinorUnits,
		TaxLabel: i.TaxLabel, TaxNote: i.TaxNote, Clients: []clientJSON{},
	}
	if i.TaxBP > 0 {
		out.TaxRate = config.FormatTaxRate(i.TaxBP)
	}
	for _, c := range i.Clients {
		out.Clients = append(out.Clients, clientJSON{c.Name, append([]string{}, c.Address...), c.TaxID})
	}
	return out
}

// saved writes changes with save and answers with the settings; a file
// that would not load is the user's input to fix, said as such.
func (s *Server) saved(r *http.Request, save func(path string) error) (any, error) {
	if err := save(s.a.ConfigPath()); err != nil {
		return nil, badRequest(strings.TrimPrefix(strings.TrimPrefix(err.Error(), "config: save settings: "), "config: save invoice details: "))
	}
	if _, err := s.a.Reload(); err != nil {
		return nil, err
	}
	return s.settings(r)
}

func (s *Server) settingsHours(r *http.Request) (any, error) {
	var b workHoursJSON
	if err := decode(r, &b); err != nil {
		return nil, err
	}
	ch := []config.Change{{Table: "hours", Key: "start"}, {Table: "hours", Key: "end"}, {Table: "hours", Key: "days"}}
	if b.Set {
		ch[0].Value, ch[1].Value, ch[2].Value = b.Start, b.End, b.Days
		if b.Days == nil {
			ch[2].Value = []string{}
		}
	}
	return s.saved(r, func(p string) error { return config.SaveValues(p, ch) })
}

func (s *Server) settingsPresets(r *http.Request) (any, error) {
	var b struct {
		Presets []int `json:"presets"`
	}
	if err := decode(r, &b); err != nil {
		return nil, err
	}
	if b.Presets == nil {
		b.Presets = []int{}
	}
	return s.saved(r, func(p string) error { return config.SaveValues(p, []config.Change{{Table: "focus", Key: "presets", Value: b.Presets}}) })
}

func (s *Server) settingsInvoice(r *http.Request) (any, error) {
	var b invoiceJSON
	if err := decode(r, &b); err != nil {
		return nil, err
	}
	// An empty field goes back to tock's default by leaving the key out.
	text := func(key, v string) config.Change {
		if v = strings.TrimSpace(v); v == "" {
			return config.Change{Table: "invoice", Key: key}
		}
		return config.Change{Table: "invoice", Key: key, Value: v}
	}
	lines := func(key string, l []string) config.Change {
		var out []string
		for _, x := range l {
			if x = strings.TrimSpace(x); x != "" {
				out = append(out, x)
			}
		}
		if len(out) == 0 {
			return config.Change{Table: "invoice", Key: key}
		}
		return config.Change{Table: "invoice", Key: key, Value: out}
	}
	ch := []config.Change{
		text("name", b.Name), lines("address", b.Address), text("currency", b.Currency), text("note", b.Note),
		text("tax_id", b.TaxID), text("round", b.Round), text("tax_label", b.TaxLabel), text("tax_rate", b.TaxRate),
		text("tax_note", b.TaxNote),
	}
	if strings.TrimSpace(b.Round) == "" || b.RoundMode == "" || b.RoundMode == "nearest" {
		ch = append(ch, config.Change{Table: "invoice", Key: "round_mode"})
	} else {
		ch = append(ch, config.Change{Table: "invoice", Key: "round_mode", Value: b.RoundMode})
	}
	if b.MinorUnits == config.DefaultInvoice().MinorUnits {
		ch = append(ch, config.Change{Table: "invoice", Key: "minor_units"})
	} else {
		ch = append(ch, config.Change{Table: "invoice", Key: "minor_units", Value: b.MinorUnits})
	}
	var clients []config.InvoiceClient
	for _, c := range b.Clients {
		var addr []string
		for _, x := range c.Address {
			if x = strings.TrimSpace(x); x != "" {
				addr = append(addr, x)
			}
		}
		clients = append(clients, config.InvoiceClient{Name: strings.TrimSpace(c.Name), Address: addr, TaxID: strings.TrimSpace(c.TaxID)})
	}
	return s.saved(r, func(p string) error { return config.SaveInvoice(p, ch, clients) })
}
