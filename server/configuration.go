package main

import (
	"fmt"
	"strconv"
	"strings"
)

type configuration struct {
	DeepgramAPIKey      string
	DeepgramRegion      string
	Model               string
	Language            string
	TranscribeAllAudio  bool
	MaxFileSizeMB       string
	MaxRecordingSeconds string
}

func defaultConfiguration() configuration {
	return configuration{DeepgramRegion: "global", Model: "nova-3", Language: "ru", TranscribeAllAudio: true, MaxFileSizeMB: "25", MaxRecordingSeconds: "300"}
}

func (c configuration) validate() error {
	if c.DeepgramRegion != "global" && c.DeepgramRegion != "eu" {
		return fmt.Errorf("DeepgramRegion must be global or eu")
	}
	if c.Model != "nova-3" && c.Model != "nova-2" {
		return fmt.Errorf("Model must be nova-3 or nova-2")
	}
	if c.Language != "ru" && c.Language != "en" && c.Language != "auto" {
		return fmt.Errorf("Language must be ru, en or auto")
	}
	if n, e := strconv.Atoi(c.MaxFileSizeMB); e != nil || n < 1 || n > 50 {
		return fmt.Errorf("MaxFileSizeMB must be 1..50")
	}
	if n, e := strconv.Atoi(c.MaxRecordingSeconds); e != nil || n < 10 || n > 600 {
		return fmt.Errorf("MaxRecordingSeconds must be 10..600")
	}
	return nil
}
func (c configuration) maxBytes() int64 {
	n, _ := strconv.ParseInt(c.MaxFileSizeMB, 10, 64)
	return n * 1024 * 1024
}
func (c configuration) seconds() int { n, _ := strconv.Atoi(c.MaxRecordingSeconds); return n }
func (p *Plugin) config() configuration {
	p.configMu.RLock()
	defer p.configMu.RUnlock()
	return p.configuration
}
func (p *Plugin) OnConfigurationChange() error {
	c := defaultConfiguration()
	if err := p.API.LoadPluginConfiguration(&c); err != nil {
		return err
	}
	c.DeepgramAPIKey = strings.TrimSpace(c.DeepgramAPIKey)
	c.MaxFileSizeMB = strings.TrimSpace(c.MaxFileSizeMB)
	c.MaxRecordingSeconds = strings.TrimSpace(c.MaxRecordingSeconds)
	defaults := defaultConfiguration()
	// Mattermost persists settings before this hook: invalid text must not brick activation.
	if n, e := strconv.Atoi(c.MaxFileSizeMB); e != nil || n < 1 || n > 50 {
		p.API.LogWarn("Invalid MaxFileSizeMB; using default 25 MiB")
		c.MaxFileSizeMB = defaults.MaxFileSizeMB
	}
	if n, e := strconv.Atoi(c.MaxRecordingSeconds); e != nil || n < 10 || n > 600 {
		p.API.LogWarn("Invalid MaxRecordingSeconds; using default 300 seconds")
		c.MaxRecordingSeconds = defaults.MaxRecordingSeconds
	}
	if c.DeepgramRegion != "global" && c.DeepgramRegion != "eu" {
		c.DeepgramRegion = defaults.DeepgramRegion
	}
	if c.Model != "nova-3" && c.Model != "nova-2" {
		c.Model = defaults.Model
	}
	if c.Language != "ru" && c.Language != "en" && c.Language != "auto" {
		c.Language = defaults.Language
	}
	p.configMu.Lock()
	p.configuration = c
	p.configMu.Unlock()
	return nil
}
