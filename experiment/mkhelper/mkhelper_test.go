package mkhelper_test

import (
	"testing"

	"github.com/ooni/probe-engine/experiment/mkhelper"
	"github.com/ooni/probe-engine/internal/mockable"
	"github.com/ooni/probe-engine/measurementkit"
	"github.com/ooni/probe-engine/model"
)

func TestNoHelpers(t *testing.T) {
	sess := &mockable.ExperimentSession{}
	var settings measurementkit.Settings
	err := mkhelper.Set(
		sess, "foobar", "https", &settings,
	)
	if err == nil {
		t.Fatal("expected an error here")
	}
}

func TestNoSuitableHelper(t *testing.T) {
	sess := &mockable.ExperimentSession{
		MockableTestHelpers: map[string][]model.Service{
			"foobar": []model.Service{
				model.Service{
					Address: "mascetti",
					Type:    "melandri",
				},
			},
		},
	}
	var settings measurementkit.Settings
	err := mkhelper.Set(
		sess, "foobar", "https", &settings,
	)
	if err == nil {
		t.Fatal("expected an error here")
	}
}

func TestGoodHelper(t *testing.T) {
	sess := &mockable.ExperimentSession{
		MockableTestHelpers: map[string][]model.Service{
			"foobar": []model.Service{
				model.Service{
					Address: "mascetti",
					Type:    "melandri",
				},
			},
		},
	}
	var settings measurementkit.Settings
	err := mkhelper.Set(
		sess, "foobar", "melandri", &settings,
	)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Options.Backend != "mascetti" {
		t.Fatal("unexpected settings.Options.Backend")
	}
}
