package pod

import (
	"testing"
	"time"

	"github.com/avereha/pod/pkg/response"
)

func runningPod(at time.Time) *PODState {
	s := &PODState{Reservoir: 3000, PodProgress: response.PodProgressRunningAbove50U, BasalActive: true}
	for i := 0; i < 48; i++ {
		s.BasalSchedule = append(s.BasalSchedule, 12) // 1.2 U/h
	}
	s.ScheduleOrigin = at.Add(-10 * time.Hour)
	s.CountedAt = at
	return s
}

// Basal is given as time passes, at the half hour's rate.
func TestBasalCountsOverTime(t *testing.T) {
	t0 := time.Now()
	s := runningPod(t0)
	s.Count(t0.Add(30 * time.Minute))
	if s.Delivered != 12 {
		t.Fatalf("half an hour at 1.2 U/h gave %d pulses, want 12", s.Delivered)
	}
	if s.Reservoir != 3000-12 {
		t.Fatalf("reservoir %d, want %d", s.Reservoir, 3000-12)
	}
}

// A temp basal replaces the schedule while it runs, then the schedule resumes.
func TestTempBasalOverridesTheSchedule(t *testing.T) {
	t0 := time.Now()
	s := runningPod(t0)
	s.TempBasalStart, s.TempBasalEnd, s.TempBasalPulses = t0, t0.Add(30*time.Minute), 0
	s.Count(t0.Add(30 * time.Minute))
	if s.Delivered != 0 {
		t.Fatalf("a zero temp gave %d pulses", s.Delivered)
	}
	s.Count(t0.Add(60 * time.Minute))
	if s.Delivered != 12 {
		t.Fatalf("after the temp, %d pulses, want 12", s.Delivered)
	}
}

// Nothing is given while delivery is stopped.
func TestNothingWhileSuspended(t *testing.T) {
	t0 := time.Now()
	s := runningPod(t0)
	s.BasalActive = false
	s.Count(t0.Add(time.Hour))
	if s.Delivered != 0 {
		t.Fatalf("suspended, %d pulses given", s.Delivered)
	}
}

// A bolus's pulses count as they are given, not at the command.
func TestABolusCountsAsItIsGiven(t *testing.T) {
	s := runningPod(time.Now())
	s.Delivered = 100
	s.BolusEnd = time.Now().Add(20 * time.Second) // 10 pulses at 2 s each, all still to come
	if got := s.DeliveredNow(); got != 100-s.BolusRemaining() || got >= 100 {
		t.Fatalf("delivered now %d with %d to come", got, s.BolusRemaining())
	}
	s.BolusEnd = time.Now().Add(-time.Second)
	if got := s.DeliveredNow(); got != 100 {
		t.Fatalf("after the bolus, delivered now %d, want 100", got)
	}
}
