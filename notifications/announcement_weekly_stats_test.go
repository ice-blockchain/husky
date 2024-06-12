// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"testing"
	stdlibtime "time"

	"github.com/stretchr/testify/assert"

	"github.com/ice-blockchain/wintr/time"
)

//nolint:funlen // .
func TestNextTimeMatch_AfterSpecifiedWeekday(t *testing.T) {
	t.Parallel()

	now := time.New(stdlibtime.Date(2024, 6, 11, 0, 0, 0, 0, stdlibtime.UTC))
	expectedTime := time.New(stdlibtime.Date(2024, 6, 17, 10, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Monday, 10, 0))
	now = time.New(stdlibtime.Date(2024, 6, 12, 15, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Monday, 10, 0))
	now = time.New(stdlibtime.Date(2024, 6, 13, 15, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Monday, 10, 0))
	now = time.New(stdlibtime.Date(2024, 6, 14, 15, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Monday, 10, 0))
	now = time.New(stdlibtime.Date(2024, 6, 15, 15, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Monday, 10, 0))
	now = time.New(stdlibtime.Date(2024, 6, 16, 15, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Monday, 10, 0))
	now = time.New(stdlibtime.Date(2024, 6, 17, 0, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Monday, 10, 0))
	now = time.New(stdlibtime.Date(2024, 6, 17, 7, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Monday, 10, 0))
	now = time.New(stdlibtime.Date(2024, 6, 17, 8, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Monday, 10, 0))
	now = time.New(stdlibtime.Date(2024, 6, 17, 9, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Monday, 10, 0))

	expectedTime = time.New(stdlibtime.Date(2024, 6, 24, 10, 0, 0, 0, stdlibtime.UTC))
	now = time.New(stdlibtime.Date(2024, 6, 17, 10, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Monday, 10, 0))
	now = time.New(stdlibtime.Date(2024, 6, 17, 10, 1, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Monday, 10, 0))
	now = time.New(stdlibtime.Date(2024, 6, 18, 0, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Monday, 10, 0))
	now = time.New(stdlibtime.Date(2024, 6, 19, 15, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Monday, 10, 0))
	now = time.New(stdlibtime.Date(2024, 6, 20, 16, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Monday, 10, 0))
	now = time.New(stdlibtime.Date(2024, 6, 21, 17, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Monday, 10, 0))
	now = time.New(stdlibtime.Date(2024, 6, 22, 17, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Monday, 10, 0))
	now = time.New(stdlibtime.Date(2024, 6, 23, 17, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Monday, 10, 0))

	expectedTime = time.New(stdlibtime.Date(2024, 7, 1, 10, 0, 0, 0, stdlibtime.UTC))
	now = time.New(stdlibtime.Date(2024, 6, 24, 17, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Monday, 10, 0))
}

//nolint:dupl // .
func TestNextTimeMatch_BeforeSpecifiedWeekday(t *testing.T) {
	t.Parallel()

	expectedTime := time.New(stdlibtime.Date(2024, 6, 17, 10, 0, 0, 0, stdlibtime.UTC))
	now := time.New(stdlibtime.Date(2024, 6, 16, 15, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Monday, 10, 0))

	now = time.New(stdlibtime.Date(2024, 6, 9, 0, 0, 0, 0, stdlibtime.UTC))
	expectedTime = time.New(stdlibtime.Date(2024, 6, 12, 12, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Wednesday, 12, 0))

	expectedTime = time.New(stdlibtime.Date(2024, 6, 12, 17, 0, 0, 0, stdlibtime.UTC))
	now = time.New(stdlibtime.Date(2024, 6, 12, 15, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Wednesday, 17, 0))

	expectedTime = time.New(stdlibtime.Date(2024, 6, 12, 17, 0, 0, 0, stdlibtime.UTC))
	now = time.New(stdlibtime.Date(2024, 6, 11, 14, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Wednesday, 17, 0))

	expectedTime = time.New(stdlibtime.Date(2024, 6, 12, 17, 0, 0, 0, stdlibtime.UTC))
	now = time.New(stdlibtime.Date(2024, 6, 10, 14, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Wednesday, 17, 0))

	expectedTime = time.New(stdlibtime.Date(2024, 6, 16, 13, 0, 0, 0, stdlibtime.UTC))
	now = time.New(stdlibtime.Date(2024, 6, 16, 8, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Sunday, 13, 0))
}

//nolint:dupl // .
func TestNextTimeMatch_SpecifiedTimeEqualToNow(t *testing.T) {
	t.Parallel()

	expectedTime := time.New(stdlibtime.Date(2024, 6, 23, 10, 0, 0, 0, stdlibtime.UTC))
	now := time.New(stdlibtime.Date(2024, 6, 16, 10, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Sunday, 10, 0))

	expectedTime = time.New(stdlibtime.Date(2024, 6, 24, 10, 0, 0, 0, stdlibtime.UTC))
	now = time.New(stdlibtime.Date(2024, 6, 17, 10, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Monday, 10, 0))

	expectedTime = time.New(stdlibtime.Date(2024, 6, 26, 11, 0, 0, 0, stdlibtime.UTC))
	now = time.New(stdlibtime.Date(2024, 6, 19, 11, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Wednesday, 11, 0))

	expectedTime = time.New(stdlibtime.Date(2024, 6, 27, 12, 0, 0, 0, stdlibtime.UTC))
	now = time.New(stdlibtime.Date(2024, 6, 20, 12, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Thursday, 12, 0))

	expectedTime = time.New(stdlibtime.Date(2024, 6, 28, 13, 0, 0, 0, stdlibtime.UTC))
	now = time.New(stdlibtime.Date(2024, 6, 21, 13, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Friday, 13, 0))

	expectedTime = time.New(stdlibtime.Date(2024, 6, 29, 14, 0, 0, 0, stdlibtime.UTC))
	now = time.New(stdlibtime.Date(2024, 6, 22, 14, 0, 0, 0, stdlibtime.UTC))
	assert.Equal(t, expectedTime, nextTimeMatch(now, stdlibtime.Saturday, 14, 0))
}
