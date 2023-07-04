// SPDX-License-Identifier: ice License 1.0

package notifications

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsBadgeNotificationDisabled_EmptyConfig(t *testing.T) {
	t.Parallel()
	cfg := config{
		DisabledAchievementsNotifications: struct {
			Badges []string "yaml:\"badges\""
			Levels []string "yaml:\"levels\""
			Roles  []string "yaml:\"roles\""
		}{},
	}

	assert.Equal(t, false, cfg.IsBadgeNotificationDisabled("b1"))
	assert.Equal(t, false, cfg.IsBadgeNotificationDisabled("b2"))
	assert.Equal(t, false, cfg.IsBadgeNotificationDisabled("b3"))
	assert.Equal(t, false, cfg.IsBadgeNotificationDisabled("b4"))
	assert.Equal(t, false, cfg.IsBadgeNotificationDisabled("b5"))
}

func TestIsRoleNotificationDisabled_EmptyConfig(t *testing.T) {
	t.Parallel()
	cfg := config{
		DisabledAchievementsNotifications: struct {
			Badges []string "yaml:\"badges\""
			Levels []string "yaml:\"levels\""
			Roles  []string "yaml:\"roles\""
		}{},
	}

	assert.Equal(t, false, cfg.IsRoleNotificationDisabled("r1"))
	assert.Equal(t, false, cfg.IsRoleNotificationDisabled("r2"))
	assert.Equal(t, false, cfg.IsRoleNotificationDisabled("r3"))
	assert.Equal(t, false, cfg.IsRoleNotificationDisabled("r4"))
	assert.Equal(t, false, cfg.IsRoleNotificationDisabled("r5"))
}

func TestIsLevelNotificationDisabled_EmptyConfig(t *testing.T) {
	t.Parallel()
	cfg := config{
		DisabledAchievementsNotifications: struct {
			Badges []string "yaml:\"badges\""
			Levels []string "yaml:\"levels\""
			Roles  []string "yaml:\"roles\""
		}{},
	}

	assert.Equal(t, false, cfg.IsLevelNotificationDisabled("l1"))
	assert.Equal(t, false, cfg.IsLevelNotificationDisabled("l2"))
	assert.Equal(t, false, cfg.IsLevelNotificationDisabled("l3"))
	assert.Equal(t, false, cfg.IsLevelNotificationDisabled("l4"))
	assert.Equal(t, false, cfg.IsLevelNotificationDisabled("l5"))
}

func TestIsBadgeNotificationDisabled(t *testing.T) {
	t.Parallel()
	cfg := config{
		DisabledAchievementsNotifications: struct {
			Badges []string "yaml:\"badges\""
			Levels []string "yaml:\"levels\""
			Roles  []string "yaml:\"roles\""
		}{
			Badges: []string{
				"b1",
				"b2",
				"b3",
				"b4",
				"b5",
			},
			Levels: []string{
				"l1",
				"l2",
				"l3",
				"l4",
				"l5",
			},
			Roles: []string{
				"r1",
				"r2",
				"r3",
				"r4",
				"r5",
			},
		},
	}

	assert.Equal(t, true, cfg.IsBadgeNotificationDisabled("b1"))
	assert.Equal(t, true, cfg.IsBadgeNotificationDisabled("b2"))
	assert.Equal(t, true, cfg.IsBadgeNotificationDisabled("b3"))
	assert.Equal(t, true, cfg.IsBadgeNotificationDisabled("b4"))
	assert.Equal(t, true, cfg.IsBadgeNotificationDisabled("b5"))
	assert.Equal(t, false, cfg.IsBadgeNotificationDisabled("b6"))
	assert.Equal(t, false, cfg.IsBadgeNotificationDisabled("b7"))
	assert.Equal(t, false, cfg.IsBadgeNotificationDisabled("b8"))
	assert.Equal(t, false, cfg.IsBadgeNotificationDisabled("b9"))
}

func TestIsRoleNotificationDisabled(t *testing.T) {
	t.Parallel()
	cfg := config{
		DisabledAchievementsNotifications: struct {
			Badges []string "yaml:\"badges\""
			Levels []string "yaml:\"levels\""
			Roles  []string "yaml:\"roles\""
		}{
			Badges: []string{
				"b1",
				"b2",
				"b3",
				"b4",
				"b5",
			},
			Levels: []string{
				"l1",
				"l2",
				"l3",
				"l4",
				"l5",
			},
			Roles: []string{
				"r1",
				"r2",
				"r3",
				"r4",
				"r5",
			},
		},
	}

	assert.Equal(t, true, cfg.IsRoleNotificationDisabled("r1"))
	assert.Equal(t, true, cfg.IsRoleNotificationDisabled("r2"))
	assert.Equal(t, true, cfg.IsRoleNotificationDisabled("r3"))
	assert.Equal(t, true, cfg.IsRoleNotificationDisabled("r4"))
	assert.Equal(t, true, cfg.IsRoleNotificationDisabled("r5"))
	assert.Equal(t, false, cfg.IsRoleNotificationDisabled("r6"))
	assert.Equal(t, false, cfg.IsRoleNotificationDisabled("r7"))
	assert.Equal(t, false, cfg.IsRoleNotificationDisabled("r8"))
	assert.Equal(t, false, cfg.IsRoleNotificationDisabled("r9"))
}

func TestIsLevelNotificationDisabled(t *testing.T) {
	t.Parallel()
	cfg := config{
		DisabledAchievementsNotifications: struct {
			Badges []string "yaml:\"badges\""
			Levels []string "yaml:\"levels\""
			Roles  []string "yaml:\"roles\""
		}{
			Badges: []string{
				"b1",
				"b2",
				"b3",
				"b4",
				"b5",
			},
			Levels: []string{
				"l1",
				"l2",
				"l3",
				"l4",
				"l5",
			},
			Roles: []string{
				"r1",
				"r2",
				"r3",
				"r4",
				"r5",
			},
		},
	}

	assert.Equal(t, true, cfg.IsLevelNotificationDisabled("l1"))
	assert.Equal(t, true, cfg.IsLevelNotificationDisabled("l2"))
	assert.Equal(t, true, cfg.IsLevelNotificationDisabled("l3"))
	assert.Equal(t, true, cfg.IsLevelNotificationDisabled("l4"))
	assert.Equal(t, true, cfg.IsLevelNotificationDisabled("l5"))
	assert.Equal(t, false, cfg.IsLevelNotificationDisabled("l6"))
	assert.Equal(t, false, cfg.IsLevelNotificationDisabled("l7"))
	assert.Equal(t, false, cfg.IsLevelNotificationDisabled("l8"))
	assert.Equal(t, false, cfg.IsLevelNotificationDisabled("l9"))
}
