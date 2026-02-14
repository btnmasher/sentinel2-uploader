package headless

import headlessview "sentinel2-uploader/internal/ui/headless/view"

// runtimeView projects mutable runtime state into the render DTO consumed by the view package.
func (m *headlessModel) runtimeView() headlessview.Runtime {
	return headlessview.Runtime{
		BuildVersion: m.buildVersion,
		Running:      m.running,
		Connecting:   m.connecting,
		Status:       m.status,
		StatusKind:   int(m.kind),
		CanConnect:   m.canConnect(),
		Channels:     m.channelHealth,
		HealthDetail: m.healthDetail,
	}
}

// View is the Bubble Tea render entrypoint; rendering is delegated to the pure view package.
func (m *headlessModel) View() string {
	return headlessview.RenderApp(&m.ui, m.runtimeView())
}
