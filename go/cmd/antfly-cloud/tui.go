package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

type tuiSnapshot struct {
	Org       Organization
	Instances []CloudInstance
	Usage     *CloudUsageSummary
	Metrics   *InstanceMetrics
	Events    []ProvisioningEvent
	Err       error
	At        time.Time
}

type tickMsg time.Time
type snapshotMsg tuiSnapshot

type tuiModel struct {
	st       *appState
	org      Organization
	selected int
	poll     time.Duration
	snap     tuiSnapshot
	loading  bool
}

func newTUICommand(stateFor stateFactory) *cobra.Command {
	var poll time.Duration
	cmd := &cobra.Command{Use: "tui", Short: "Monitor Antfly Cloud status in a terminal UI", RunE: func(cmd *cobra.Command, args []string) error {
		st, err := stateFor(true)
		if err != nil {
			return err
		}
		org, err := resolveOrg(cmd.Context(), st)
		if err != nil {
			return err
		}
		m := tuiModel{st: st, org: org, poll: poll, loading: true}
		_, err = tea.NewProgram(m).Run()
		return err
	}}
	cmd.Flags().DurationVar(&poll, "poll", 5*time.Second, "poll interval")
	return cmd
}

func (m tuiModel) Init() tea.Cmd { return tea.Batch(m.fetch(), tick(m.poll)) }

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m tuiModel) fetch() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), m.st.timeout)
		defer cancel()
		s := tuiSnapshot{Org: m.org, At: time.Now()}
		instances, err := m.st.client.Instances(ctx, m.org.ID)
		if err != nil {
			s.Err = err
			return snapshotMsg(s)
		}
		s.Instances = instances
		usage, err := m.st.client.Usage(ctx, m.org.ID)
		if err != nil {
			s.Err = err
			return snapshotMsg(s)
		}
		s.Usage = usage
		if len(instances) > 0 {
			idx := m.selected
			if idx >= len(instances) {
				idx = len(instances) - 1
			}
			metrics, err := m.st.client.Metrics(ctx, m.org.ID, instances[idx].ID)
			if err == nil {
				s.Metrics = metrics
			}
			events, err := m.st.client.Events(ctx, m.org.ID, instances[idx].ID)
			if err == nil {
				s.Events = events
			}
		}
		return snapshotMsg(s)
	}
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			if m.selected > 0 {
				m.selected--
				m.loading = true
				return m, m.fetch()
			}
		case "down", "j":
			if m.selected < len(m.snap.Instances)-1 {
				m.selected++
				m.loading = true
				return m, m.fetch()
			}
		case "r":
			m.loading = true
			return m, m.fetch()
		}
	case tickMsg:
		m.loading = true
		return m, tea.Batch(m.fetch(), tick(m.poll))
	case snapshotMsg:
		m.snap = tuiSnapshot(msg)
		m.loading = false
		if m.selected >= len(m.snap.Instances) && len(m.snap.Instances) > 0 {
			m.selected = len(m.snap.Instances) - 1
		}
	}
	return m, nil
}

func (m tuiModel) View() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Antfly Cloud — %s (%s)\n", m.org.Name, m.org.Slug)
	if !m.snap.At.IsZero() {
		fmt.Fprintf(&b, "Last refresh: %s", m.snap.At.Local().Format("15:04:05"))
	}
	if m.loading {
		b.WriteString("  refreshing…")
	}
	b.WriteString("\n")
	if m.snap.Err != nil {
		fmt.Fprintf(&b, "Error: %v\n", m.snap.Err)
	}
	if m.snap.Usage != nil {
		fmt.Fprintf(&b, "Usage: queries=%d cpu=%.2fh memory=%.2fGiBh storage=%.2fGiBh\n", m.snap.Usage.Totals.Queries, m.snap.Usage.Totals.CPUCoreHours, m.snap.Usage.Totals.MemoryGiBHours, m.snap.Usage.Totals.StorageGiBHours)
	}
	b.WriteString("\nInstances\n")
	if len(m.snap.Instances) == 0 {
		b.WriteString("  No cloud instances.\n")
	}
	for i, inst := range m.snap.Instances {
		cursor := " "
		if i == m.selected {
			cursor = ">"
		}
		fmt.Fprintf(&b, "%s %-24s %-14s %-12s %-10s %s\n", cursor, inst.Name, inst.Slug, inst.Status, inst.Region, shortID(inst.ID))
	}
	if len(m.snap.Instances) > 0 {
		inst := m.snap.Instances[m.selected]
		fmt.Fprintf(&b, "\nSelected: %s (%s)\n", inst.Name, inst.ID)
		fmt.Fprintf(&b, "Status: %s  Mode: %s  Updated: %s\n", inst.Status, inst.Mode, fmtTime(inst.UpdatedAt))
		if m.snap.Metrics != nil {
			fmt.Fprintf(&b, "Metrics: tables=%d docs=%d storage=%s queries_this_month=%d nodes=%d\n", m.snap.Metrics.TableCount, m.snap.Metrics.DocumentCount, bytesHuman(m.snap.Metrics.StorageUsedBytes), m.snap.Metrics.QueriesThisMonth, m.snap.Metrics.NodeCount)
		}
		b.WriteString("Recent events:\n")
		limit := len(m.snap.Events)
		if limit > 5 {
			limit = 5
		}
		for i := 0; i < limit; i++ {
			e := m.snap.Events[i]
			fmt.Fprintf(&b, "  %s  %-20s %s\n", fmtTime(e.CreatedAt), e.EventType, e.Message)
		}
	}
	b.WriteString("\n↑/↓ select  r refresh  q quit\n")
	return b.String()
}
