package server

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/dhamith93/SyMon/internal/api"
)

// Alert rules: every user can see them, admins can change them. A rule has
// the same fields as an alerts.json entry.

type alertRule struct {
	ID        int64           `json:"id"`
	Enabled   bool            `json:"enabled"`
	Rule      json.RawMessage `json:"rule"`
	UpdatedAt int64           `json:"updatedAt"`
	// empty for rules SyMon set up
	UpdatedBy string `json:"updatedBy"`
}

// requireAdmin answers 403 for viewers. It runs behind requireLogin.
func (s *server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		session, err := s.sessionOf(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, errNotLoggedIn.Error())
			return
		}
		if session.role != "admin" {
			writeError(w, http.StatusForbidden, "only admins can change alert rules")
			return
		}
		next(w, r)
	}
}

func (s *server) getRules(w http.ResponseWriter, r *http.Request) {
	response, err := s.collector.AlertRules(r.Context(), &api.Void{})
	if err != nil {
		writeGRPCError(w, "alert rules", err)
		return
	}
	rules := make([]alertRule, 0, len(response.Rules))
	for _, rule := range response.Rules {
		rules = append(rules, alertRule{
			ID:        rule.Id,
			Enabled:   rule.Enabled,
			Rule:      json.RawMessage(rule.RuleJson),
			UpdatedAt: rule.UpdatedAt,
			UpdatedBy: rule.UpdatedBy,
		})
	}
	writeJSON(w, map[string]any{"rules": rules})
}

func (s *server) postRule(w http.ResponseWriter, r *http.Request) {
	s.saveRule(w, r, 0)
}

func (s *server) putRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, "no such rule")
		return
	}
	s.saveRule(w, r, id)
}

// saveRule creates a rule for id 0, and replaces one otherwise
func (s *server) saveRule(w http.ResponseWriter, r *http.Request, id int64) {
	if !jsonBody(r) {
		writeError(w, http.StatusUnsupportedMediaType, "send the rule as JSON")
		return
	}
	var body struct {
		Enabled bool            `json:"enabled"`
		Rule    json.RawMessage `json:"rule"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&body); err != nil || len(body.Rule) == 0 {
		writeError(w, http.StatusBadRequest, "send enabled and rule")
		return
	}
	session, _ := s.sessionOf(r)
	saved, err := s.collector.SaveRule(r.Context(), &api.SaveRuleRequest{Id: id, Enabled: body.Enabled, RuleJson: string(body.Rule), By: session.user})
	if err != nil {
		writeGRPCError(w, "saving a rule", err)
		return
	}
	if id == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]int64{"id": saved.Id})
		return
	}
	writeJSON(w, map[string]int64{"id": saved.Id})
}

func (s *server) deleteRule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusNotFound, "no such rule")
		return
	}
	session, _ := s.sessionOf(r)
	if _, err := s.collector.DeleteRule(r.Context(), &api.RuleRequest{Id: id, By: session.user}); err != nil {
		writeGRPCError(w, "deleting a rule", err)
		return
	}
	writeJSON(w, map[string]int64{"id": id})
}
