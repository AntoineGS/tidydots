package tui

import (
	"errors"
	"fmt"
	"io"

	tea "charm.land/bubbletea/v2"
)

type configRestoreItem struct {
	item        SubEntryItem
	name        string
	forceRender bool
}

type configRestoreMsg struct {
	queued  configRestoreItem
	success bool
	message string
}

type configRestoreExec struct {
	stdout  io.Writer
	model   Model
	queued  configRestoreItem
	success bool
	message string
}

func (e *configRestoreExec) SetStdin(io.Reader)    {}
func (e *configRestoreExec) SetStdout(w io.Writer) { e.stdout = w }
func (e *configRestoreExec) SetStderr(io.Writer)   {}

func (e *configRestoreExec) Run() error {
	if e.stdout != nil {
		_, _ = fmt.Fprintf(e.stdout, "\nRestoring: %s/%s\n", e.queued.item.AppName, e.queued.item.SubEntry.Name)
	}
	e.success, e.message = e.model.performConfigRestore(e.queued.item, e.queued.forceRender)
	if !e.success {
		return errors.New(e.message)
	}
	return nil
}

func (m *Model) startConfigRestores(items []configRestoreItem, setups []setupRunItem) tea.Cmd {
	if len(m.pendingConfigRestores) != 0 {
		return nil
	}
	m.pendingConfigRestores = items
	m.configRestoreSetups = setups
	m.processing = true
	return m.runNextConfigRestore()
}

func (m Model) runNextConfigRestore() tea.Cmd {
	if len(m.pendingConfigRestores) == 0 {
		return nil
	}
	queued := m.pendingConfigRestores[0]
	exec := &configRestoreExec{model: m, queued: queued}
	return tea.Exec(exec, func(err error) tea.Msg {
		success, message := exec.success, exec.message
		if err != nil {
			success = false
			if message == "" {
				message = fmt.Sprintf("Failed: %v", err)
			}
		}
		return configRestoreMsg{queued: queued, success: success, message: message}
	})
}

func (m Model) handleConfigRestoreResult(msg configRestoreMsg) (tea.Model, tea.Cmd) {
	m.results = append(m.results, ResultItem{Name: msg.queued.name, Success: msg.success, Message: msg.message})
	if msg.success {
		m.configRestoreDirty = true
	}
	if msg.success && msg.queued.item.Generation == m.WorkspaceGeneration {
		for appIdx := range m.Applications {
			for subIdx := range m.Applications[appIdx].SubItems {
				row := &m.Applications[appIdx].SubItems[subIdx]
				if row.Ref == msg.queued.item.Ref && row.Generation == msg.queued.item.Generation {
					row.State = StateLoading
					row.CheckError = ""
				}
			}
		}
	}
	if len(m.pendingConfigRestores) > 0 {
		m.pendingConfigRestores = m.pendingConfigRestores[1:]
	}
	if len(m.pendingConfigRestores) > 0 {
		return m, m.runNextConfigRestore()
	}
	setups := m.configRestoreSetups
	m.configRestoreSetups = nil
	m.processing = false
	if m.configRestoreDirty && m.Workspace != nil {
		focus := msg.queued.item.Ref
		if err := m.refreshWorkspace(&focus, msg.queued.item.AppName); err != nil {
			m.results = append(m.results, ResultItem{Name: "workspace refresh", Success: false, Message: err.Error()})
		}
	} else if m.configRestoreDirty {
		m.refreshApplicationStates()
	}
	m.configRestoreDirty = false
	m.rebuildTable()
	if len(setups) > 0 {
		return m, m.startSetupRun(setups, false)
	}
	m.Screen = ScreenResults
	m.Operation = OpList
	m.showingResults = true
	m.resultsScrollOffset = 0
	return m, m.dispatchLoadingSubEntryStates()
}
