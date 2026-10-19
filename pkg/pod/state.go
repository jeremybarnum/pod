package pod

import (
	"io/ioutil"
	"time"

	toml "github.com/pelletier/go-toml"
	log "github.com/sirupsen/logrus"

	"github.com/avereha/pod/pkg/response"
)

type PODState struct {
	LTK       []byte `toml:"ltk"`
	EapAkaSeq uint64 `toml:"eap_aka_seq"`

	Id []byte `toml:"id"` // 4 byte

	MsgSeq   uint8  `toml:"msg_seq"`   // TODO: is this the same as nonceSeq?
	CmdSeq   uint8  `toml:"cmd_seq"`   // TODO: are all those 3 the same number ???
	NonceSeq uint64 `toml:"nonce_seq"` // or 16?

	LastProgSeqNum uint8 `toml:"last_prog_seq"`

	NoncePrefix []byte `toml:"nonce_prefix"`
	CK          []byte `toml:"ck"`

	PodProgress    response.PodProgress
	ActivationTime time.Time `toml:"activation_time"`

	// Delivery over time, as a pod meters it: the basal schedule (pulses per
	// half hour, from ScheduleOrigin), a temp basal's pulses per half hour
	// from TempBasalStart, the fraction of a pulse not yet given, and when
	// delivery was last counted.
	BasalSchedule   []uint16  `toml:"basal_schedule"`
	ScheduleOrigin  time.Time `toml:"schedule_origin"`
	TempBasalPulses float64   `toml:"temp_basal_pulses"`
	TempBasalStart  time.Time `toml:"temp_basal_start"`
	PulseFraction   float64   `toml:"pulse_fraction"`
	CountedAt       time.Time `toml:"counted_at"`
	BolusNotGiven   uint16    `toml:"bolus_not_given"` // a cancelled bolus's pulses

	Reservoir        uint16 `toml:"reservoir"`
	ActiveAlertSlots uint8  `toml:"alerts"`
	FaultEvent       uint8  `toml:"fault"`
	FaultTime        uint16 `toml:"fault_time"`
	Delivered        uint16 `toml:"delivered"`

	TriggerTimes     [8]uint16 `toml:"trigger_times"`

	// At some point these could be replaced with details
	// of each kind of delivery (volume, start time, schedule, etc)
	BolusEnd            time.Time `toml:"bolus_end"`
	BolusCanceledAt     time.Time `toml:"bolus_canceled_at"`
	TempBasalEnd        time.Time `toml:"temp_basal_end"`
	ExtendedBolusActive bool      `toml:"extended_bolus_active"`
	BasalActive         bool      `toml:"basal_active"`

	// PeripheralID is the identifier the iOS Simulator transport gives the pod.
	PeripheralID string `toml:"peripheral_id"`

	Filename string
}

// NewFreshState is a new pod, not yet paired.
func NewFreshState(filename string) *PODState {
	return &PODState{
		Reservoir:      150 / 0.05,
		ActivationTime: time.Now(),
		Filename:       filename,
	}
}

func NewState(filename string) (*PODState, error) {
	var ret PODState
	ret.Filename = filename
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	err = toml.Unmarshal(data, &ret)
	if err != nil {
		return nil, err
	}
	return &ret, nil
}

func (p *PODState) Save() error {
	log.Debugf("Saving state to file: %s", p.Filename)
	data, err := toml.Marshal(p)
	if err != nil {
		return err
	}
	return ioutil.WriteFile(p.Filename, data, 0777)
}

func (p *PODState) MinutesActive() uint16 {
	return uint16(time.Now().Sub(p.ActivationTime).Round(time.Minute).Minutes())
}

// Count adds the basal and temp basal pulses given since the last count, as
// the pod's pump does: by the half hour's rate, a temp basal over the schedule,
// nothing while delivery is stopped or before the cannula is in.
func (p *PODState) Count(now time.Time) {
	if p.CountedAt.IsZero() || p.PodProgress < response.PodProgressRunningAbove50U || p.PodProgress > response.PodProgressRunningBelow50U {
		p.CountedAt = now
		return
	}
	t := p.CountedAt
	for t.Before(now) {
		next := now
		var perSecond float64
		switch {
		case p.TempBasalEnd.After(t):
			if p.TempBasalEnd.Before(next) {
				next = p.TempBasalEnd
			}
			perSecond = p.TempBasalPulses / 1800
		case p.BasalActive && len(p.BasalSchedule) > 0:
			into := t.Sub(p.ScheduleOrigin)
			half := int(into/(30*time.Minute)) % len(p.BasalSchedule)
			if half < 0 {
				half += len(p.BasalSchedule)
			}
			boundary := p.ScheduleOrigin.Add((into/(30*time.Minute) + 1) * 30 * time.Minute)
			if into < 0 {
				boundary = t.Add(30 * time.Minute)
			}
			if boundary.Before(next) {
				next = boundary
			}
			perSecond = float64(p.BasalSchedule[half]) / 1800
		}
		p.PulseFraction += perSecond * next.Sub(t).Seconds()
		t = next
	}
	whole := uint16(p.PulseFraction)
	p.PulseFraction -= float64(whole)
	p.Delivered += whole
	if p.Reservoir >= whole {
		p.Reservoir -= whole
	}
	p.CountedAt = now
}

// DeliveredNow is the pulses given so far: what the pump has counted, less a
// bolus's pulses still to come.
func (p *PODState) DeliveredNow() uint16 {
	if p.BolusEnd.After(time.Now()) {
		return p.Delivered - p.BolusRemaining()
	}
	return p.Delivered
}

// NOTE: only handles immediate boluses; any extended bolus is not accounted for
func (p *PODState) BolusRemaining() uint16 {
	now := time.Now()
	var secondsPerPulse uint16
	if p.BolusEnd.After(now) {
		// Add one so the response for a bolus command has a bolus remaining value that matches the bolus size
		bolusSecondsRemaining := uint16(p.BolusEnd.Sub(now).Seconds() + 1)
		if p.PodProgress > response.PodProgressInsertingCannula {
			secondsPerPulse = 2 // normal immediate bolus rate
		} else {
			secondsPerPulse = 1 // pod setup bolus rate
		}
		return bolusSecondsRemaining / secondsPerPulse
	} else {
		return p.BolusNotGiven
	}
}
