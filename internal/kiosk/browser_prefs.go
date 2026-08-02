package kiosk

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func configureChromeProfile(profileDir, certDER string) error {
	if err := writeUserDataPolicies(profileDir, certDER); err != nil {
		return err
	}

	defaultDir := filepath.Join(profileDir, "Default")
	if err := os.MkdirAll(defaultDir, 0o755); err != nil {
		return err
	}
	prefsPath := filepath.Join(defaultDir, "Preferences")
	prefs := map[string]any{}
	if b, err := os.ReadFile(prefsPath); err == nil && len(b) > 0 {
		_ = json.Unmarshal(b, &prefs)
	}
	prefs["credentials_enable_service"] = false
	prefs["credentials_enable_autosignin"] = false
	prefs["alternate_error_pages"] = map[string]any{"enabled": false}
	prefs["autofill"] = map[string]any{
		"profile_enabled":     false,
		"credit_card_enabled": false,
		"enabled":             false,
	}
	prefs["browser"] = map[string]any{
		"custom_chrome_frame":   false,
		"check_default_browser": false,
		"has_seen_welcome_page": true,
		"show_home_button":      false,
		"enable_spellchecking":  false,
	}
	prefs["distribution"] = map[string]any{
		"import_bookmarks":          false,
		"import_history":            false,
		"import_search_engine":      false,
		"make_chrome_default":       false,
		"skip_first_run_ui":         true,
		"suppress_first_run_bubble": true,
	}
	prefs["dns_prefetching"] = map[string]any{"enabled": false}
	prefs["download"] = map[string]any{
		"prompt_for_download": false,
		"directory_upgrade":   true,
	}
	prefs["translate"] = map[string]any{"enabled": false}
	prefs["safebrowsing"] = map[string]any{
		"enabled":  true,
		"enhanced": false,
	}
	prefs["search"] = map[string]any{"suggest_enabled": false}
	prefs["net"] = map[string]any{"network_prediction_options": 2}
	prefs["printing"] = map[string]any{
		"enabled":                false,
		"print_preview_disabled": true,
	}
	prefs["webkit"] = map[string]any{
		"webprefs": map[string]any{
			"tabs_to_links": false,
		},
	}
	signin, _ := prefs["signin"].(map[string]any)
	if signin == nil {
		signin = map[string]any{}
	}
	signin["allowed"] = false
	prefs["signin"] = signin
	profile, _ := prefs["profile"].(map[string]any)
	if profile == nil {
		profile = map[string]any{}
	}
	profile["password_manager_enabled"] = false
	profile["exit_type"] = "Normal"
	profile["exited_cleanly"] = true
	profile["default_content_setting_values"] = map[string]any{
		"popups":              2,
		"automatic_downloads": 2,
		"notifications":       2,
		"geolocation":         2,
		"media_stream":        2,
		"media_stream_mic":    2,
		"media_stream_camera": 2,
		"midi_sysex":          2,
		"protocol_handlers":   2,
		"ppapi_broker":        2,
		"clipboard":           2,
		"sensors":             2,
		"payment_handler":     2,
		"idle_detection":      2,
		"window_placement":    2,
	}
	prefs["profile"] = profile
	session, _ := prefs["session"].(map[string]any)
	if session == nil {
		session = map[string]any{}
	}
	session["restore_on_startup"] = 5
	prefs["session"] = session
	bookmarkBar, _ := prefs["bookmark_bar"].(map[string]any)
	if bookmarkBar == nil {
		bookmarkBar = map[string]any{}
	}
	bookmarkBar["show_on_all_tabs"] = false
	prefs["bookmark_bar"] = bookmarkBar
	b, err := json.MarshalIndent(prefs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(prefsPath, b, 0o644)
}

func kioskManagedPolicies(certDERBase64 string) map[string]any {
	return map[string]any{
		"DeveloperToolsAvailability":   2,
		"BrowserGuestModeEnabled":      false,
		"BrowserAddPersonEnabled":      false,
		"IncognitoModeAvailability":    1,
		"BookmarkBarEnabled":           false,
		"EditBookmarksEnabled":         false,
		"PasswordManagerEnabled":       false,
		"AutofillAddressEnabled":       false,
		"AutofillCreditCardEnabled":    false,
		"PrintingEnabled":              false,
		"DownloadRestrictions":         3,
		"PromptForDownloadLocation":    false,
		"TranslateEnabled":             false,
		"DefaultPopupsSetting":         2,
		"DefaultNotificationsSetting":  2,
		"DefaultGeolocationSetting":    2,
		"DefaultMediaStreamSetting":    2,
		"AudioCaptureAllowed":          false,
		"VideoCaptureAllowed":          false,
		"SavingBrowserHistoryDisabled": true,
		"SearchSuggestEnabled":         false,
		"AlternateErrorPagesEnabled":   false,
		"NetworkPredictionOptions":     2,
		"BrowserSignin":                0,
		"SyncDisabled":                 true,
		"AllowDinosaurEasterEgg":       false,
		"DisableScreenshots":           true,
		"FullscreenAllowed":            true,
		"PasswordLeakDetectionEnabled": false,
		"CACertificates":               []string{certDERBase64},
	}
}

func writeUserDataPolicies(userDataDir, certDER string) error {
	return writePolicyFile(
		filepath.Join(userDataDir, "Policy", "Managed", "kiosk.json"),
		kioskManagedPolicies(certDER),
	)
}

func writePolicyFile(path string, policies map[string]any) error {
	b, err := json.MarshalIndent(policies, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
