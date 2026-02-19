package qos

import (
	"fmt"

	"github.com/veesix-networks/osvbng/pkg/vpp/binapi/policer_types"
)

type Action uint8

const (
	ActionDrop            Action = 0
	ActionTransmit        Action = 1
	ActionMarkAndTransmit Action = 2
)

func (a *Action) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	switch s {
	case "drop":
		*a = ActionDrop
	case "transmit":
		*a = ActionTransmit
	case "mark-and-transmit":
		*a = ActionMarkAndTransmit
	default:
		return fmt.Errorf("unknown action %q", s)
	}
	return nil
}

type ActionConfig struct {
	Action Action `yaml:"action"`
	DSCP   uint8  `yaml:"dscp,omitempty"`
}

type Policy struct {
	CIR     uint32       `yaml:"cir"`
	EIR     uint32       `yaml:"eir,omitempty"`
	CBS     uint64       `yaml:"cbs,omitempty"`
	EBS     uint64       `yaml:"ebs,omitempty"`
	Conform ActionConfig `yaml:"conform"`
	Exceed  ActionConfig `yaml:"exceed"`
	Violate ActionConfig `yaml:"violate"`
}

func (p *Policy) Defaults() {
	if p.EIR == 0 {
		p.EIR = p.CIR
	}
	if p.CBS == 0 {
		p.CBS = uint64(p.CIR) * 1000 / 8
	}
	if p.EBS == 0 {
		p.EBS = p.CBS
	}
}

func (p *Policy) ToPolicerConfig() policer_types.PolicerConfig {
	p.Defaults()
	return policer_types.PolicerConfig{
		Cir:       p.CIR,
		Eir:       p.EIR,
		Cb:        p.CBS,
		Eb:        p.EBS,
		RateType:  policer_types.SSE2_QOS_RATE_API_KBPS,
		RoundType: policer_types.SSE2_QOS_ROUND_API_TO_CLOSEST,
		Type:      policer_types.SSE2_QOS_POLICER_TYPE_API_2R3C_RFC_2698,
		ConformAction: policer_types.Sse2QosAction{
			Type: policer_types.Sse2QosActionType(p.Conform.Action),
			Dscp: p.Conform.DSCP,
		},
		ExceedAction: policer_types.Sse2QosAction{
			Type: policer_types.Sse2QosActionType(p.Exceed.Action),
			Dscp: p.Exceed.DSCP,
		},
		ViolateAction: policer_types.Sse2QosAction{
			Type: policer_types.Sse2QosActionType(p.Violate.Action),
			Dscp: p.Violate.DSCP,
		},
	}
}
