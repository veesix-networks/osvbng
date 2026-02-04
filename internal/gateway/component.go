package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"

	pb "github.com/veesix-networks/osvbng/api/proto"
	"github.com/veesix-networks/osvbng/internal/subscriber"
	"github.com/veesix-networks/osvbng/pkg/component"
	"github.com/veesix-networks/osvbng/pkg/configmgr"
	"github.com/veesix-networks/osvbng/pkg/handlers/conf"
	"github.com/veesix-networks/osvbng/pkg/handlers/oper"
	"github.com/veesix-networks/osvbng/pkg/handlers/show"
	"github.com/veesix-networks/osvbng/pkg/logger"
	"github.com/veesix-networks/osvbng/pkg/models"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/show/aaa"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/show/ip"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/show/protocols/bgp"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/show/subscriber"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/show/system"
	_ "github.com/veesix-networks/osvbng/pkg/handlers/show/vrf"
	"google.golang.org/grpc"
)

type Component struct {
	*component.Base
	pb.UnimplementedBNGServiceServer

	logger       *slog.Logger
	server       *grpc.Server
	showRegistry *show.Registry
	operRegistry *oper.Registry
	bindAddr     string
	subscriber   *subscriber.Component
	configd      *configmgr.ConfigManager
}

func New(deps component.Dependencies, showRegistry *show.Registry, operRegistry *oper.Registry, subscriberComp *subscriber.Component, configd *configmgr.ConfigManager, bindAddr string) (component.Component, error) {
	return &Component{
		Base:         component.NewBase("gateway"),
		logger:       logger.Component(logger.ComponentGateway),
		showRegistry: showRegistry,
		operRegistry: operRegistry,
		bindAddr:     bindAddr,
		subscriber:   subscriberComp,
		configd:      configd,
	}, nil
}

func (c *Component) Start(ctx context.Context) error {
	c.StartContext(ctx)
	c.logger.Info("Starting gateway component", "addr", c.bindAddr)

	lis, err := net.Listen("tcp", c.bindAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	c.server = grpc.NewServer()
	pb.RegisterBNGServiceServer(c.server, c)

	c.logger.Info("Gateway started", "addr", c.bindAddr)

	go func() {
		if err := c.server.Serve(lis); err != nil {
			c.logger.Error("Gateway server error", "error", err)
		}
	}()

	return nil
}

func (c *Component) Stop(ctx context.Context) error {
	c.logger.Info("Stopping gateway component")

	if c.server != nil {
		c.server.GracefulStop()
	}

	c.StopContext()
	return nil
}

func (c *Component) GetOperationalStats(ctx context.Context, req *pb.GetOperationalStatsRequest) (*pb.GetOperationalStatsResponse, error) {
	handler, err := c.showRegistry.GetHandler(req.Path)
	if err != nil {
		return nil, err
	}

	data, err := handler.Collect(ctx, &show.Request{
		Path: req.Path,
	})
	if err != nil {
		return nil, err
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal data: %w", err)
	}

	return &pb.GetOperationalStatsResponse{
		Metrics: map[string][]byte{
			req.Path: jsonData,
		},
	}, nil
}

func (c *Component) ExecuteOperation(ctx context.Context, req *pb.ExecuteOperationRequest) (*pb.ExecuteOperationResponse, error) {
	handler, err := c.operRegistry.GetHandler(req.Path)
	if err != nil {
		return nil, err
	}

	data, err := handler.Execute(ctx, &oper.Request{
		Path: req.Path,
		Body: req.Body,
	})
	if err != nil {
		return nil, err
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal data: %w", err)
	}

	return &pb.ExecuteOperationResponse{
		Data: jsonData,
	}, nil
}

func (c *Component) GetRunningConfig(ctx context.Context, req *pb.GetRunningConfigRequest) (*pb.ConfigResponse, error) {
	if c.configd == nil {
		return nil, fmt.Errorf("configd not available")
	}

	config, err := c.configd.GetRunning()
	if err != nil {
		return nil, err
	}

	yamlBytes, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}

	return &pb.ConfigResponse{
		ConfigYaml: string(yamlBytes),
	}, nil
}

func (c *Component) GetStartupConfig(ctx context.Context, req *pb.GetStartupConfigRequest) (*pb.ConfigResponse, error) {
	if c.configd == nil {
		return nil, fmt.Errorf("configd not available")
	}

	config, err := c.configd.GetStartup()
	if err != nil {
		return nil, err
	}

	yamlBytes, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}

	return &pb.ConfigResponse{
		ConfigYaml: string(yamlBytes),
	}, nil
}

func (c *Component) ListVersions(ctx context.Context, req *pb.ListVersionsRequest) (*pb.ListVersionsResponse, error) {
	if c.configd == nil {
		return nil, fmt.Errorf("configd not available")
	}

	versions, err := c.configd.ListVersions()
	if err != nil {
		return nil, err
	}

	pbVersions := make([]*pb.ConfigVersion, 0, len(versions))
	for _, v := range versions {
		changes := make([]*pb.ConfigChange, 0, len(v.Changes))
		for _, ch := range v.Changes {
			value := ""
			if ch.Value != nil {
				if val, ok := ch.Value.(string); ok {
					value = val
				} else {
					valueBytes, _ := json.Marshal(ch.Value)
					value = string(valueBytes)
				}
			}
			changes = append(changes, &pb.ConfigChange{
				Type:  ch.Type,
				Path:  ch.Path,
				Value: value,
			})
		}

		pbVersions = append(pbVersions, &pb.ConfigVersion{
			Version:   int32(v.Version),
			Timestamp: v.Timestamp.Unix(),
			CommitMsg: v.CommitMsg,
			Changes:   changes,
		})
	}

	return &pb.ListVersionsResponse{
		Versions: pbVersions,
	}, nil
}

func (c *Component) GetVersion(ctx context.Context, req *pb.GetVersionRequest) (*pb.VersionResponse, error) {
	if c.configd == nil {
		return nil, fmt.Errorf("configd not available")
	}

	version, err := c.configd.GetVersion(int(req.Version))
	if err != nil {
		return nil, err
	}

	changes := make([]*pb.ConfigChange, 0, len(version.Changes))
	for _, ch := range version.Changes {
		value := ""
		if ch.Value != nil {
			if v, ok := ch.Value.(string); ok {
				value = v
			} else {
				valueBytes, _ := json.Marshal(ch.Value)
				value = string(valueBytes)
			}
		}
		changes = append(changes, &pb.ConfigChange{
			Type:  ch.Type,
			Path:  ch.Path,
			Value: value,
		})
	}

	return &pb.VersionResponse{
		Version: &pb.ConfigVersion{
			Version:   int32(version.Version),
			Timestamp: version.Timestamp.Unix(),
			CommitMsg: version.CommitMsg,
			Changes:   changes,
		},
	}, nil
}

func (c *Component) ConfigEnter(ctx context.Context, req *pb.ConfigEnterRequest) (*pb.ConfigEnterResponse, error) {
	if c.configd == nil {
		return nil, fmt.Errorf("configd not available")
	}

	sessionID, err := c.configd.CreateCandidateSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create config session: %w", err)
	}

	return &pb.ConfigEnterResponse{
		SessionId: string(sessionID),
	}, nil
}

func (c *Component) ConfigSet(ctx context.Context, req *pb.ConfigSetRequest) (*pb.ConfigSetResponse, error) {
	if c.configd == nil {
		return nil, fmt.Errorf("configd not available")
	}

	sessionID := conf.SessionID(req.SessionId)

	if err := c.configd.Set(sessionID, req.Path, req.Value); err != nil {
		return &pb.ConfigSetResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	return &pb.ConfigSetResponse{
		Success: true,
		Message: "Configuration updated",
	}, nil
}

func (c *Component) ConfigCommit(ctx context.Context, req *pb.ConfigCommitRequest) (*pb.ConfigCommitResponse, error) {
	if c.configd == nil {
		return nil, fmt.Errorf("configd not available")
	}

	sessionID := conf.SessionID(req.SessionId)

	if err := c.configd.Commit(sessionID); err != nil {
		return &pb.ConfigCommitResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	versions, _ := c.configd.ListVersions()
	latestVersion := int32(0)
	if len(versions) > 0 {
		latestVersion = int32(versions[len(versions)-1].Version)
	}

	return &pb.ConfigCommitResponse{
		Success: true,
		Message: "Configuration committed successfully",
		Version: latestVersion,
	}, nil
}

func (c *Component) ConfigDiscard(ctx context.Context, req *pb.ConfigDiscardRequest) (*pb.ConfigDiscardResponse, error) {
	if c.configd == nil {
		return nil, fmt.Errorf("configd not available")
	}

	sessionID := conf.SessionID(req.SessionId)

	if err := c.configd.CloseCandidateSession(sessionID); err != nil {
		return &pb.ConfigDiscardResponse{
			Success: false,
		}, err
	}

	return &pb.ConfigDiscardResponse{
		Success: true,
	}, nil
}

func (c *Component) ConfigDiff(ctx context.Context, req *pb.ConfigDiffRequest) (*pb.ConfigDiffResponse, error) {
	if c.configd == nil {
		return nil, fmt.Errorf("configd not available")
	}

	sessionID := conf.SessionID(req.SessionId)

	diff, err := c.configd.DryRun(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to get diff: %w", err)
	}

	added := make([]*pb.DiffLine, 0, len(diff.Added))
	for _, line := range diff.Added {
		added = append(added, &pb.DiffLine{
			Path:  line.Path,
			Value: line.Value,
		})
	}

	deleted := make([]*pb.DiffLine, 0, len(diff.Deleted))
	for _, line := range diff.Deleted {
		deleted = append(deleted, &pb.DiffLine{
			Path:  line.Path,
			Value: line.Value,
		})
	}

	modified := make([]*pb.DiffLine, 0, len(diff.Modified))
	for _, line := range diff.Modified {
		modified = append(modified, &pb.DiffLine{
			Path:  line.Path,
			Value: line.Value,
		})
	}

	return &pb.ConfigDiffResponse{
		Added:    added,
		Deleted:  deleted,
		Modified: modified,
	}, nil
}

func (c *Component) GetSessions(ctx context.Context, req *pb.GetSessionsRequest) (*pb.GetSessionsResponse, error) {
	handler, err := c.showRegistry.GetHandler("subscriber.sessions")
	if err != nil {
		return nil, err
	}

	options := make(map[string]string)
	if req.AccessType != "" {
		options["access_type"] = req.AccessType
	}
	if req.Protocol != "" {
		options["protocol"] = req.Protocol
	}
	if req.Svlan > 0 {
		options["svlan"] = fmt.Sprintf("%d", req.Svlan)
	}

	data, err := handler.Collect(ctx, &show.Request{
		Path:    "subscriber.sessions",
		Options: options,
	})
	if err != nil {
		return nil, err
	}

	sessions, ok := data.([]models.SubscriberSession)
	if !ok {
		return nil, fmt.Errorf("invalid data type from handler")
	}

	pbSessions := make([]*pb.Session, 0, len(sessions))
	for _, sess := range sessions {
		pbSess := &pb.Session{
			SessionId:     sess.GetSessionID(),
			AccessType:    string(sess.GetAccessType()),
			Protocol:      string(sess.GetProtocol()),
			Mac:           sess.GetMAC().String(),
			OuterVlan:     uint32(sess.GetOuterVLAN()),
			InnerVlan:     uint32(sess.GetInnerVLAN()),
			State:         string(sess.GetState()),
			AcctSessionId: sess.GetRADIUSSessionID(),
			SwIfIndex:     sess.GetIfIndex(),
		}

		if ipv4 := sess.GetIPv4Address(); ipv4 != nil {
			pbSess.Ipv4Address = ipv4.String()
		}
		if ipv6 := sess.GetIPv6Address(); ipv6 != nil {
			pbSess.Ipv6Address = ipv6.String()
		}

		if dhcp4, ok := sess.(*models.DHCPv4Session); ok {
			pbSess.Hostname = dhcp4.Hostname
			pbSess.LeaseTime = dhcp4.LeaseTime
		}

		pbSessions = append(pbSessions, pbSess)
	}

	return &pb.GetSessionsResponse{Sessions: pbSessions}, nil
}

func (c *Component) GetSession(ctx context.Context, req *pb.GetSessionRequest) (*pb.Session, error) {
	handler, err := c.showRegistry.GetHandler("subscriber.session")
	if err != nil {
		return nil, err
	}

	data, err := handler.Collect(ctx, &show.Request{
		Path: "subscriber.session",
		Options: map[string]string{
			"session_id": req.SessionId,
		},
	})
	if err != nil {
		return nil, err
	}

	sess, ok := data.(*models.DHCPv4Session)
	if !ok {
		return nil, fmt.Errorf("invalid data type from handler")
	}

	return &pb.Session{
		SessionId:     sess.SessionID,
		AccessType:    sess.AccessType,
		Protocol:      sess.Protocol,
		Mac:           sess.MAC.String(),
		OuterVlan:     uint32(sess.OuterVLAN),
		InnerVlan:     uint32(sess.InnerVLAN),
		State:         string(sess.State),
		Ipv4Address:   sess.IPv4Address.String(),
		Hostname:      sess.Hostname,
		SwIfIndex:     uint32(sess.IfIndex),
		LeaseTime:     sess.LeaseTime,
		AcctSessionId: sess.RADIUSSessionID,
	}, nil
}

func (c *Component) TerminateSession(ctx context.Context, req *pb.TerminateSessionRequest) (*pb.TerminateSessionResponse, error) {
	if err := c.subscriber.TerminateSession(ctx, req.SessionId); err != nil {
		return &pb.TerminateSessionResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	return &pb.TerminateSessionResponse{
		Success: true,
		Message: "Session terminated successfully",
	}, nil
}

func (c *Component) GetStats(ctx context.Context, req *pb.GetStatsRequest) (*pb.Stats, error) {
	stats, err := c.subscriber.GetStats(ctx)
	if err != nil {
		return nil, err
	}

	return &pb.Stats{
		TotalSessions:    stats["total"],
		IpoeV4Sessions:   stats["ipoe_v4"],
		IpoeV6Sessions:   stats["ipoe_v6"],
		PppSessions:      stats["ppp"],
		ActiveSessions:   stats["active"],
		ReleasedSessions: stats["released"],
	}, nil
}
