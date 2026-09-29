package api

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"

	"github.com/dhamith93/SyMon/internal/alerts"
	"github.com/dhamith93/SyMon/internal/logger"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Server) AlertRules(ctx context.Context, in *Void) (*AlertRuleList, error) {
	rules, err := s.Store.AlertRules(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	list := &AlertRuleList{}
	for _, rule := range rules {
		data, err := json.Marshal(rule.Rule)
		if err != nil {
			return nil, toStatus(err)
		}
		list.Rules = append(list.Rules, &AlertRuleInfo{
			Id:        rule.ID,
			Enabled:   rule.Enabled,
			RuleJson:  string(data),
			UpdatedAt: rule.UpdatedAt.Unix(),
			UpdatedBy: rule.UpdatedBy,
		})
	}
	return list, nil
}

// SaveRule creates a rule for id 0 and replaces one otherwise. Unknown
// fields are refused, like in alerts.json.
func (s *Server) SaveRule(ctx context.Context, in *SaveRuleRequest) (*AlertRuleInfo, error) {
	var rule alerts.AlertConfig
	decoder := json.NewDecoder(bytes.NewReader([]byte(in.RuleJson)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&rule); err != nil {
		return nil, status.Error(codes.InvalidArgument, "cannot read the rule: "+err.Error())
	}

	id := in.Id
	var err error
	if id == 0 {
		id, err = s.Store.CreateRule(ctx, rule, in.Enabled, in.By)
	} else {
		err = s.Store.UpdateRule(ctx, id, rule, in.Enabled, in.By)
	}
	if err != nil {
		return nil, toStatus(err)
	}
	logger.Log("info", "alert rule "+strconv.Quote(rule.Name)+" saved by "+strconv.Quote(in.By))
	return &AlertRuleInfo{Id: id, Enabled: in.Enabled, UpdatedBy: in.By}, nil
}

func (s *Server) DeleteRule(ctx context.Context, in *RuleRequest) (*Message, error) {
	if err := s.Store.DeleteRule(ctx, in.Id); err != nil {
		return nil, toStatus(err)
	}
	logger.Log("info", "alert rule "+strconv.FormatInt(in.Id, 10)+" deleted by "+strconv.Quote(in.By))
	return &Message{Body: "ok"}, nil
}
