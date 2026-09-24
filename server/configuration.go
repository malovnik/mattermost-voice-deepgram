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
	if err := c.validate(); err != nil {
		return err
	}
	p.configMu.Lock()
	p.configuration = c
	p.configMu.Unlock()
	return nil
}
