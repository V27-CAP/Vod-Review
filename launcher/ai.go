package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

type AISettings struct {
	Provider string `json:"provider"`
	Endpoint string `json:"endpoint"`
	Model    string `json:"model"`
	APIKey   string `json:"api_key"`
	Vision   bool   `json:"vision"`
}

type aiProviderItem struct {
	id       string
	pt, en   string
	endpoint string
	model    string
	cloud    bool
}

var aiProviders = []aiProviderItem{
	{id: "lmstudio", pt: "LM Studio (IA Local)", en: "LM Studio (Local AI)", endpoint: "http://127.0.0.1:1234/v1/chat/completions", model: "local-model", cloud: false},
	{id: "ollama", pt: "Ollama (IA Local)", en: "Ollama (Local AI)", endpoint: "http://127.0.0.1:11434/v1/chat/completions", model: "llama3.2", cloud: false},
	{id: "gemini", pt: "Google Gemini (Nuvem)", en: "Google Gemini (Cloud)", endpoint: "https://generativelanguage.googleapis.com/v1beta", model: "gemini-2.5-flash", cloud: true},
	{id: "openai", pt: "OpenAI (Nuvem)", en: "OpenAI (Cloud)", endpoint: "https://api.openai.com/v1/chat/completions", model: "gpt-4o-mini", cloud: true},
}

func defaultAISettings() AISettings {
	p := aiProviders[0]
	return AISettings{Provider: p.id, Endpoint: p.endpoint, Model: p.model}
}

func providerByID(id string) (aiProviderItem, int) {
	for i, p := range aiProviders {
		if p.id == id {
			return p, i
		}
	}
	return aiProviders[0], 0
}

func isDefaultEndpoint(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return true
	}
	for _, p := range aiProviders {
		if v == p.endpoint {
			return true
		}
	}
	return false
}

func isDefaultModel(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" {
		return true
	}
	for _, p := range aiProviders {
		if v == p.model {
			return true
		}
	}
	return false
}

func loadAISettings(path string) AISettings {
	cfg := defaultAISettings()
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &cfg)
	}
	p, _ := providerByID(cfg.Provider)
	cfg.Provider = p.id
	if strings.TrimSpace(cfg.Endpoint) == "" {
		cfg.Endpoint = p.endpoint
	}
	if strings.TrimSpace(cfg.Model) == "" {
		cfg.Model = p.model
	}
	return cfg
}

func saveAISettings(path string, cfg AISettings) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600)
}

func (u *desk) aiSummary() string {
	p, _ := providerByID(u.ai.Provider)
	label := u.t(p.pt, p.en)
	model := strings.TrimSpace(u.ai.Model)
	if model == "" {
		model = p.model
	}
	summary := fmt.Sprintf("%s · %s", label, model)
	if u.ai.Vision {
		summary += u.t(" · Visão ativa", " · Vision on")
	}
	return summary
}

func (u *desk) aiConfigDialog(owner walk.Form) bool {
	t := u.t
	if owner == nil {
		owner = u.mw
	}
	var dlg *walk.Dialog
	var okBtn, cancelBtn *walk.PushButton
	var providerBox *walk.ComboBox
	var endpointEdit, modelEdit, keyEdit *walk.LineEdit
	var visionBox *walk.CheckBox
	var hintLabel *walk.Label

	labels := make([]string, len(aiProviders))
	for i, p := range aiProviders {
		labels[i] = t(p.pt, p.en)
	}
	curProv, curIdx := providerByID(u.ai.Provider)
	updateHint := func(p aiProviderItem) {
		if hintLabel == nil {
			return
		}
		if p.cloud {
			hintLabel.SetText(t(
				"Nuvem: informe sua chave de API abaixo (ou use GEMINI_API_KEY / OPENAI_API_KEY).",
				"Cloud: enter your API key below (or set GEMINI_API_KEY / OPENAI_API_KEY).",
			))
		} else {
			hintLabel.SetText(t(
				"Local: mantenha o LM Studio ou Ollama rodando em 127.0.0.1. Chave de API é opcional.",
				"Local: keep LM Studio or Ollama running on 127.0.0.1. API key is optional.",
			))
		}
	}

	err := u.dialog(Dialog{
		AssignTo:      &dlg,
		Title:         t("Configurar IA (Local e Nuvem)", "Configure AI (Local & Cloud)"),
		DefaultButton: &okBtn,
		CancelButton:  &cancelBtn,
		MinSize:       Size{Width: 480},
		Layout:        VBox{Margins: Margins{18, 14, 18, 14}, Spacing: 8},
		Children: []Widget{
			Label{Text: t("Provedor de IA para categorias, contexto, descrição e hashtags", "AI provider for categories, context, description and hashtags"), Font: font(11, true)},
			Composite{Layout: Grid{Columns: 2, MarginsZero: true}, Children: []Widget{
				Label{Text: t("Provedor", "Provider")},
				ComboBox{
					AssignTo:     &providerBox,
					Model:        labels,
					CurrentIndex: curIdx,
					OnCurrentIndexChanged: func() {
						idx := providerBox.CurrentIndex()
						if idx < 0 || idx >= len(aiProviders) || endpointEdit == nil || modelEdit == nil {
							return
						}
						p := aiProviders[idx]
						if isDefaultEndpoint(endpointEdit.Text()) {
							endpointEdit.SetText(p.endpoint)
						}
						if isDefaultModel(modelEdit.Text()) {
							modelEdit.SetText(p.model)
						}
						updateHint(p)
					},
				},
				Label{Text: t("Endpoint / URL", "Endpoint / URL")},
				LineEdit{AssignTo: &endpointEdit, Text: u.ai.Endpoint},
				Label{Text: t("Modelo", "Model")},
				LineEdit{AssignTo: &modelEdit, Text: u.ai.Model},
				Label{Text: t("Chave de API", "API Key")},
				LineEdit{AssignTo: &keyEdit, Text: u.ai.APIKey, PasswordMode: true},
			}},
			Label{AssignTo: &hintLabel, Text: "", NoPrefix: true},
			CheckBox{
				AssignTo: &visionBox,
				Checked:  u.ai.Vision,
				Text:     t("Enviar recortes da facecam para modelos com visão (quando facecam estiver ativa)", "Send facecam crops to vision-capable models (when facecam is enabled)"),
			},
			Composite{Layout: HBox{MarginsZero: true}, Children: []Widget{
				PushButton{Text: t("Restaurar padrão do provedor", "Reset provider defaults"), OnClicked: func() {
					idx := providerBox.CurrentIndex()
					if idx < 0 || idx >= len(aiProviders) {
						idx = 0
					}
					p := aiProviders[idx]
					endpointEdit.SetText(p.endpoint)
					modelEdit.SetText(p.model)
				}},
				HSpacer{},
				PushButton{AssignTo: &cancelBtn, Text: t("Cancelar", "Cancel"), OnClicked: func() { dlg.Cancel() }},
				PushButton{AssignTo: &okBtn, Text: t("Salvar IA", "Save AI"), OnClicked: func() {
					idx := providerBox.CurrentIndex()
					if idx < 0 || idx >= len(aiProviders) {
						idx = 0
					}
					p := aiProviders[idx]
					ep := strings.TrimSpace(endpointEdit.Text())
					model := strings.TrimSpace(modelEdit.Text())
					if ep == "" {
						ep = p.endpoint
					}
					if model == "" {
						model = p.model
					}
					u.ai = AISettings{
						Provider: p.id,
						Endpoint: ep,
						Model:    model,
						APIKey:   strings.TrimSpace(keyEdit.Text()),
						Vision:   visionBox.Checked(),
					}
					if err := saveAISettings(u.aiPrefs, u.ai); err != nil {
						walk.MsgBox(dlg, title, err.Error(), walk.MsgBoxIconWarning)
						return
					}
					dlg.Accept()
				}},
			}},
		},
	}).Create(owner)
	if err != nil {
		u.error(err)
		return false
	}
	updateHint(curProv)
	return dlg.Run() == walk.DlgCmdOK
}
