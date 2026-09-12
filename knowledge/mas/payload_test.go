package mas_test

import (
	"math"
	"testing"
	"time"

	"github.com/memoria-space/meking/knowledge"
	"github.com/memoria-space/meking/knowledge/mas"
)

func TestPayloadRequiresObservedKnowledge(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, subject := range []knowledge.ObjectRef{knowledge.EntityID(id), knowledge.RelationID(id), knowledge.ClaimID(id)} {
		payload := mas.Payload{OccuredAt: at, Subject: subject, Grade: mas.GradeGood, Version: 1}
		if err := payload.Validate(); err != nil {
			t.Fatal(err)
		}
	}
	valid := mas.Payload{OccuredAt: at, Subject: knowledge.EntityID(id), Grade: mas.GradeGood, Version: 1}
	for name, change := range map[string]func(*mas.Payload){
		"missing time":     func(p *mas.Payload) { p.OccuredAt = time.Time{} },
		"missing subject":  func(p *mas.Payload) { p.Subject = nil },
		"invalid entity":   func(p *mas.Payload) { p.Subject = knowledge.EntityID("invalid") },
		"invalid relation": func(p *mas.Payload) { p.Subject = knowledge.RelationID("invalid") },
		"invalid claim":    func(p *mas.Payload) { p.Subject = knowledge.ClaimID("invalid") },
		"missing version":  func(p *mas.Payload) { p.Version = 0 },
		"missing grade":    func(p *mas.Payload) { p.Grade = 0 },
		"invalid grade":    func(p *mas.Payload) { p.Grade = 5 },
	} {
		t.Run(name, func(t *testing.T) {
			payload := valid
			change(&payload)
			if err := payload.Validate(); err == nil {
				t.Fatalf("accepted invalid payload %+v", payload)
			}
		})
	}
}

func TestMemoryValueBounds(t *testing.T) {
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -1} {
		if _, err := mas.NewStability(value); err == nil {
			t.Fatalf("accepted stability %g", value)
		}
		if _, err := mas.NewDifficulty(value); err == nil {
			t.Fatalf("accepted difficulty %g", value)
		}
		if _, err := mas.NewActivation(value); err == nil {
			t.Fatalf("accepted activation %g", value)
		}
	}
	for _, value := range []float64{0, 0.00099} {
		if _, err := mas.NewStability(value); err == nil {
			t.Fatalf("accepted stability %g", value)
		}
	}
	for _, value := range []float64{0.001, 36500} {
		if _, err := mas.NewStability(value); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []float64{1, 10} {
		if _, err := mas.NewDifficulty(value); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []float64{0, 11} {
		if _, err := mas.NewDifficulty(value); err == nil {
			t.Fatalf("accepted difficulty %g", value)
		}
	}
	for _, value := range []float64{0, 1} {
		if _, err := mas.NewActivation(value); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := mas.NewActivation(1.01); err == nil {
		t.Fatal("accepted activation above one")
	}
}
