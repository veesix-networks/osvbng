package system

import (
	"context"
	"encoding/json"

	"github.com/veesix-networks/osvbng/pkg/deps"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/handlers/show/paths"
	"github.com/veesix-networks/osvbng/pkg/opdb"
)

type OpDBSessionsHandler struct {
	deps *deps.ShowDeps
}

type OpDBSession struct {
	Namespace string                 `json:"namespace"`
	Key       string                 `json:"key"`
	Data      map[string]interface{} `json:"data"`
}

func init() {
	show.RegisterFactory(func(deps *deps.ShowDeps) show.ShowHandler {
		return &OpDBSessionsHandler{deps: deps}
	})
}

func (h *OpDBSessionsHandler) Collect(ctx context.Context, req *show.Request) (interface{}, error) {
	if h.deps.OpDB == nil {
		return []OpDBSession{}, nil
	}

	var sessions []OpDBSession

	namespaces := []string{
		opdb.NamespaceIPoESessions,
		opdb.NamespacePPPoESessions,
	}

	nsFilter := req.Options["namespace"]

	for _, ns := range namespaces {
		if nsFilter != "" && ns != nsFilter {
			continue
		}

		err := h.deps.OpDB.Load(ctx, ns, func(key string, value []byte) error {
			var data map[string]interface{}
			if err := json.Unmarshal(value, &data); err != nil {
				data = map[string]interface{}{"raw": string(value)}
			}

			sessions = append(sessions, OpDBSession{
				Namespace: ns,
				Key:       key,
				Data:      data,
			})
			return nil
		})
		if err != nil {
			continue
		}
	}

	return sessions, nil
}

func (h *OpDBSessionsHandler) PathPattern() paths.Path {
	return paths.SystemOpDBSessions
}

func (h *OpDBSessionsHandler) Dependencies() []paths.Path {
	return nil
}
