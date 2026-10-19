package command

import (
	"github.com/avereha/pod/pkg/response"
	log "github.com/sirupsen/logrus"
)

type ProgramInsulin struct {
	Seq      uint8
	ID       []byte
	TableNum byte
	Pulses   uint16
	Duration uint8 // Number of half hour increments; for a basal schedule, the current half hour
	// Pulses per half hour, one per half hour of the table: the basal
	// schedule's 48, or the temp basal's duration.
	Schedule []uint16
}

func UnmarshalProgramInsulin(data []byte) (*ProgramInsulin, error) {
	ret := &ProgramInsulin{}
	log.Debugf("ProgramInsulin, 0x1a, received, data %x", data)

	// 1a LL NNNNNNNN 02 CCCC HH SSSS PPPP 0ppp
	//    00 01020304 05 0607 08 0910 1112 1314
	ret.TableNum = data[5]
	ret.Duration = data[8]
	ret.Pulses = (uint16(data[11]) << 8) + uint16(data[12])
	// The insulin schedule table, after the first segment's pulses: entries of
	// napp (n+1 half hours of pp pulses, a: alternate half hours get one more),
	// up to the command's length.
	end := 1 + int(data[0])
	if end > len(data) {
		end = len(data)
	}
	for i := 13; i+1 < end; i += 2 {
		e := (uint16(data[i]) << 8) | uint16(data[i+1])
		n, alt, pp := int(e>>12)+1, e&0x0800 != 0, e&0x03FF
		for k := 0; k < n; k++ {
			v := pp
			if alt && k%2 == 1 {
				v++
			}
			ret.Schedule = append(ret.Schedule, v)
		}
	}
	return ret, nil
}

func (g *ProgramInsulin) GetSeq() uint8 {
	return g.Seq
}

func (g *ProgramInsulin) IsResponseHardcoded() bool {
	return false
}

func (g *ProgramInsulin) DoesMutatePodState() bool {
	return true
}

func (g *ProgramInsulin) GetResponse() (response.Response, error) {
	return &response.GeneralStatusResponse{}, nil
}

func (g *ProgramInsulin) SetHeaderData(seq uint8, id []byte) error {
	g.ID = id
	g.Seq = seq
	return nil
}

func (g *ProgramInsulin) GetHeaderData() (uint8, []byte, error) {
	return g.Seq, g.ID, nil
}

func (g *ProgramInsulin) GetPayload() Payload {
	return nil
}

func (g *ProgramInsulin) GetType() Type {
	return PROGRAM_INSULIN
}
