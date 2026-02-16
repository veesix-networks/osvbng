package vpp

import (
	"context"
	"fmt"
	"github.com/veesix-networks/osvbng/pkg/models/system"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/vlib"
	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/vpe"
)

func (v *VPP) GetVersion(ctx context.Context) (string, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return "", err
	}
	defer ch.Close()

	reply := &vpe.ShowVersionReply{}
	if err := ch.SendRequest(&vpe.ShowVersion{}).ReceiveReply(reply); err != nil {
		return "", err
	}

	return fmt.Sprintf("%s %s", reply.Program, reply.Version), nil
}


func (v *VPP) GetSystemThreads() ([]system.Thread, error) {
	ch, err := v.conn.NewAPIChannel()
	if err != nil {
		return []system.Thread{}, fmt.Errorf("create API channel: %w", err)
	}
	defer ch.Close()

	req := &vlib.ShowThreads{}
	reply := &vlib.ShowThreadsReply{}

	if err := ch.SendRequest(req).ReceiveReply(reply); err != nil {
		return []system.Thread{}, fmt.Errorf("GetSystemThreads(): %w", err)
	}

	if reply.Retval != 0 {
		return []system.Thread{}, fmt.Errorf("failed to gather system threads: retval=%d", reply.Retval)
	}

	var systemThreads []system.Thread
	for _, t := range reply.ThreadData {
		systemThreads = append(systemThreads, system.Thread{
			ID:        t.ID,
			Name:      t.Name,
			Type:      t.Type,
			ProcessID: t.PID,
			CPUID:     t.CPUID,
			CPUCore:   t.Core,
			CPUSocket: t.CPUSocket,
		})
	}

	return systemThreads, nil
}


