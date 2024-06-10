// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"fmt"
	stdlog "log"
	"strings"
	stdlibtime "time"

	"github.com/rcrowley/go-metrics"

	"github.com/ice-blockchain/wintr/log"
)

func init() { //nolint:gochecknoinits // .
	stdlog.SetFlags(stdlog.Ldate | stdlog.Ltime | stdlog.Lmsgprefix | stdlog.LUTC | stdlog.Lmicroseconds)
}

type (
	telemetry struct {
		registry        metrics.Registry
		currentStepName string
		steps           []string
	}
)

func (t *telemetry) mustInit(steps []string) *telemetry {
	const (
		decayAlpha    = 0.015
		reservoirSize = 10_000
	)
	t.registry = metrics.NewRegistry()
	t.steps = append(t.steps, steps...)
	for ix := range t.steps {
		if ix > 1 {
			t.steps[ix] = fmt.Sprintf("[%v]scheduled.%v", ix-1, t.steps[ix])
		}
		log.Panic(t.registry.Register(t.steps[ix], metrics.NewCustomTimer(metrics.NewHistogram(metrics.NewExpDecaySample(reservoirSize, decayAlpha)), metrics.NewMeter()))) //nolint:lll // .
	}

	go metrics.LogScaled(t.registry, 15*stdlibtime.Minute, stdlibtime.Millisecond, t) //nolint:gomnd,mnd // .

	return t
}

func (t *telemetry) collectElapsed(step int, since stdlibtime.Time) {
	t.registry.Get(t.steps[step]).(metrics.Timer).UpdateSince(since) //nolint:forcetypeassert // .
}

func (t *telemetry) Printf(format string, args ...any) {
	const prefixMarker = "timer "
	if strings.HasPrefix(format, prefixMarker) {
		t.currentStepName = fmt.Sprintf(format[len(prefixMarker):strings.IndexRune(format, '\n')], args...)
	}
	stdlog.Printf("["+t.currentStepName+"]"+strings.ReplaceAll(format, prefixMarker, ""), args...)
}
