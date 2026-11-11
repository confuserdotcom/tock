package web

import (
	"fmt"
	"net/http"

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

type settingsJSON struct {
	Features    []featureJSON `json:"features"`
	HideApps    []string      `json:"hideApps"`
	HideSites   []string      `json:"hideSites"`
	ConfigPath  string        `json:"configPath"`
	ConfigError string        `json:"configError,omitempty"`
}

func (s *Server) settings(*http.Request) (any, error) {
	cfg := s.a.Config()
	out := settingsJSON{
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
