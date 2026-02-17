package view

import "fmt"

const (
	zoneTabOverview = "tab-overview"
	zoneTabSettings = "tab-settings"

	zoneOverviewConnect   = "overview-connect"
	zoneOverviewLogs      = "overview-logs"
	zoneOverviewQuit      = "overview-quit"
	zoneOverviewLogsDebug = "overview-logs-debug"

	zoneSettingsBrowse      = "settings-browse"
	zoneSettingsAutoConnect = "settings-auto-connect"
	zoneSettingsSave        = "settings-save"
	zoneSettingsCancel      = "settings-cancel"

	zoneDialogQuitCancel  = "dialog-quit-cancel"
	zoneDialogQuitAccept  = "dialog-quit-accept"
	zoneDialogUpdateLater = "dialog-update-later"
	zoneDialogUpdateOpen  = "dialog-update-open"
)

func zoneSettingsInput(index int) string {
	return fmt.Sprintf("settings-input-%d", index)
}
