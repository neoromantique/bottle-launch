// TUI model: state management and update handlers using the Bubbletea framework.
package main

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

type viewState int

const (
	viewBottleList viewState = iota
	viewBottleActions
	viewPermissions
	viewAppSelect
	viewLaunchConfirm
	viewPasswordInput
	viewCreateBottle
	viewDeleteConfirm
	viewRunning
	viewError
	viewCreateBottleYubiKey // YubiKey bottle creation wizard
	viewFIDO2Unlock         // Touch to unlock
)

type model struct {
	state     viewState
	prevState viewState

	// Components
	help    help.Model
	keys    keyMap
	spinner spinner.Model

	// Bottle selection
	bottles        []string
	bottleList     list.Model
	selectedBottle string

	// App selection
	apps        []FlatpakApp
	appList     list.Model
	selectedApp FlatpakApp

	// Permissions
	permissions *Permissions
	configPath  string

	// Generic cursor for menu navigation (reused across views)
	cursor int

	// Forms
	createForm    *huh.Form
	passwordInput textinput.Model
	password      string

	// Error handling
	err    error
	errMsg string

	// Mount info for cleanup
	mountInfo *MountInfo

	// Window size
	width  int
	height int

	// Loading state
	loading    bool
	loadingMsg string

	// FIDO2
	fido2Devices      []FIDO2Device
	fido2DeviceSel    int    // selected device index
	fido2Step         int    // wizard step (0-6)
	fido2BottleID     string // temp storage during creation
	fido2CredID       string // temp storage during creation
	fido2Salt         string // temp storage during creation
	fido2Secret       []byte // temp: derived secret (cleared after use)
	fido2Error        string // last error message
	bottleUsesYubiKey bool   // loaded from config

	// YubiKey bottle creation form values
	fido2BottleName string
	fido2BottleSize string
}

func initialModel() model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle

	ti := textinput.New()
	ti.Placeholder = "Enter password"
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '*'
	ti.Focus()

	bottles := listBottles()
	bottleItems := make([]list.Item, len(bottles))
	for i, b := range bottles {
		// Check if this is a YubiKey bottle
		configPath := getConfigPath(b)
		perms := loadPermissions(configPath)
		isYubiKey, _ := IsFIDO2Bottle(perms)
		bottleItems[i] = bottleItem{path: b, name: bottleName(b), isYubiKey: isYubiKey}
	}

	bl := list.New(bottleItems, bottleItemDelegate{}, 40, 15)
	bl.Title = "Select Bottle"
	bl.SetShowStatusBar(false)
	bl.SetFilteringEnabled(false)
	bl.Styles.Title = titleStyle
	bl.SetShowHelp(false)

	return model{
		state:         viewBottleList,
		help:          help.New(),
		keys:          defaultKeyMap(),
		spinner:       s,
		bottles:       bottles,
		bottleList:    bl,
		passwordInput: ti,
		permissions:   defaultPermissions(),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, tea.EnterAltScreen)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.Width = msg.Width
		m.bottleList.SetSize(msg.Width-4, msg.Height-8)
		if m.state == viewAppSelect {
			m.appList.SetSize(msg.Width-4, msg.Height-8)
		}
		return m, nil

	case tea.KeyMsg:
		// Global quit handling - works from anywhere
		switch msg.String() {
		case "ctrl+c":
			// Unmount before quitting
			if m.mountInfo != nil {
				udisksUnmountBottle(m.mountInfo)
				m.mountInfo = nil
				SetCurrentMountInfo(nil)
			}
			return m, tea.Quit
		case "q":
			// 'q' quits except during text input or forms
			if m.state != viewPasswordInput && m.state != viewCreateBottle {
				// Unmount before quitting
				if m.mountInfo != nil {
					udisksUnmountBottle(m.mountInfo)
					m.mountInfo = nil
					SetCurrentMountInfo(nil)
				}
				return m, tea.Quit
			}
		}

	case errMsg:
		m.err = msg.err
		m.errMsg = msg.err.Error()
		m.state = viewError
		m.loading = false
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case bottlesLoadedMsg:
		m.bottles = msg.bottles
		items := make([]list.Item, len(msg.bottles))
		for i, b := range msg.bottles {
			// Check if this is a YubiKey bottle
			configPath := getConfigPath(b)
			perms := loadPermissions(configPath)
			isYubiKey, _ := IsFIDO2Bottle(perms)
			items[i] = bottleItem{path: b, name: bottleName(b), isYubiKey: isYubiKey}
		}
		m.bottleList.SetItems(items)
		m.loading = false
		return m, nil

	case appsLoadedMsg:
		m.apps = msg.apps
		items := make([]list.Item, len(msg.apps))
		for i, app := range msg.apps {
			items[i] = appItem{app: app}
		}
		al := list.New(items, appItemDelegate{}, m.width-4, m.height-8)
		al.Title = "Select Application"
		al.SetShowStatusBar(false)
		al.SetFilteringEnabled(true)
		al.Styles.Title = titleStyle
		al.SetShowHelp(false)
		m.appList = al
		m.state = viewAppSelect
		m.loading = false
		return m, nil

	case mountSuccessMsg:
		m.mountInfo = msg.info
		SetCurrentMountInfo(msg.info) // Update global for signal handler
		m.loading = false
		m.state = viewRunning
		return m, runFlatpakCmd(m.selectedApp.ID, msg.info.MountPoint, m.permissions, nil)

	case mountFailedMsg:
		m.loading = false
		if msg.wrongPassword {
			m.errMsg = "Wrong password. Please try again."
			m.passwordInput.Reset()
			m.state = viewPasswordInput
		} else {
			m.err = msg.err
			m.errMsg = msg.err.Error()
			m.state = viewError
		}
		return m, nil

	case appFinishedMsg:
		// App finished running, unmount and return to bottle list
		if m.mountInfo != nil {
			if err := udisksUnmountBottle(m.mountInfo); err != nil {
				m.errMsg = "Unmount failed: " + err.Error()
				m.state = viewError
				m.mountInfo = nil
				SetCurrentMountInfo(nil)
				return m, nil
			}
			m.mountInfo = nil
			SetCurrentMountInfo(nil) // Clear global
		}
		m.state = viewBottleList
		return m, loadBottlesCmd()

	case bottleCreatedMsg:
		m.loading = false
		m.state = viewBottleList
		return m, loadBottlesCmd()

	case bottleDeletedMsg:
		m.loading = false
		m.state = viewBottleList
		return m, loadBottlesCmd()

	case fido2DevicesMsg:
		m.fido2Devices = msg.devices
		m.fido2Error = ""
		m.loading = false
		if msg.err != nil {
			m.fido2Error = msg.err.Error()
		}
		return m, nil

	case fido2CredentialCreatedMsg:
		m.loading = false
		if msg.err != nil {
			m.fido2Error = msg.err.Error()
			return m, nil
		}
		m.fido2CredID = msg.credID
		m.fido2Salt = msg.salt
		m.fido2Step = 2 // Move to "get secret" step
		m.fido2Error = ""
		return m, nil

	case fido2SecretReadyMsg:
		m.loading = false
		if msg.err != nil {
			m.fido2Error = msg.err.Error()
			return m, nil
		}
		m.fido2Secret = msg.secret
		m.fido2Step = 3 // Move to "create bottle" step
		m.fido2Error = ""
		return m, nil

	case fido2BottleCreatedMsg:
		// Clear sensitive data
		m.fido2Secret = nil
		m.loading = false
		if msg.err != nil {
			m.fido2Error = msg.err.Error()
			return m, nil
		}
		m.fido2Step = 4 // Success step
		m.fido2Error = ""
		return m, nil

	case fido2UnlockSuccessMsg:
		m.mountInfo = msg.info
		SetCurrentMountInfo(msg.info) // Update global for signal handler
		m.loading = false
		m.fido2Secret = nil // Clear sensitive data
		m.state = viewRunning
		return m, runFlatpakCmd(m.selectedApp.ID, msg.info.MountPoint, m.permissions, nil)

	case fido2UnlockFailedMsg:
		m.loading = false
		m.fido2Secret = nil
		m.fido2Error = msg.err.Error()
		m.state = viewFIDO2Unlock
		return m, nil
	}

	// Delegate to current view
	switch m.state {
	case viewBottleList:
		return m.updateBottleList(msg)
	case viewBottleActions:
		return m.updateBottleActions(msg)
	case viewPermissions:
		return m.updatePermissions(msg)
	case viewAppSelect:
		return m.updateAppSelect(msg)
	case viewLaunchConfirm:
		return m.updateLaunchConfirm(msg)
	case viewPasswordInput:
		return m.updatePasswordInput(msg)
	case viewCreateBottle:
		return m.updateCreateBottle(msg)
	case viewDeleteConfirm:
		return m.updateDeleteConfirm(msg)
	case viewError:
		return m.updateError(msg)
	case viewCreateBottleYubiKey:
		return m.updateCreateBottleYubiKey(msg)
	case viewFIDO2Unlock:
		return m.updateFIDO2Unlock(msg)
	}

	return m, nil
}

func (m model) updateBottleList(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if i, ok := m.bottleList.SelectedItem().(bottleItem); ok {
				m.selectedBottle = i.path
				m.configPath = getConfigPath(i.path)
				m.permissions = loadPermissions(m.configPath)
				m.cursor = 0
				m.state = viewBottleActions
				return m, nil
			}
		case "n", "+":
			// New bottle (password)
			m.createForm = createBottleForm()
			m.state = viewCreateBottle
			return m, m.createForm.Init()
		case "y":
			// New bottle (YubiKey)
			m.fido2Step = 0
			m.fido2BottleName = ""
			m.fido2BottleSize = ""
			m.fido2Devices = nil
			m.fido2DeviceSel = 0
			m.fido2BottleID = ""
			m.fido2CredID = ""
			m.fido2Salt = ""
			m.fido2Secret = nil
			m.fido2Error = ""
			m.createForm = createBottleFormYubiKey()
			m.state = viewCreateBottleYubiKey
			return m, m.createForm.Init()
		case "?":
			// Could show help - for now just continue
		}
	}

	var cmd tea.Cmd
	m.bottleList, cmd = m.bottleList.Update(msg)
	return m, cmd
}

func (m model) updateBottleActions(msg tea.Msg) (tea.Model, tea.Cmd) {
	const numActions = 3

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.state = viewBottleList
			return m, nil
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < numActions-1 {
				m.cursor++
			}
		case "enter":
			switch m.cursor {
			case 0: // Launch
				m.loading = true
				m.loadingMsg = "Loading applications..."
				return m, loadAppsCmd()
			case 1: // Permissions
				m.cursor = 0
				m.state = viewPermissions
				return m, nil
			case 2: // Delete
				m.state = viewDeleteConfirm
				return m, nil
			}
		case "l", "1":
			m.loading = true
			m.loadingMsg = "Loading applications..."
			return m, loadAppsCmd()
		case "p", "2":
			m.cursor = 0
			m.state = viewPermissions
			return m, nil
		case "d", "3":
			m.state = viewDeleteConfirm
			return m, nil
		}
	}
	return m, nil
}

func (m model) updatePermissions(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "enter":
			// Save and go back
			savePermissions(m.configPath, m.permissions)
			m.cursor = 0
			m.state = viewBottleActions
			return m, nil
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(permissionDefs)-1 {
				m.cursor++
			}
		case " ":
			// Toggle current permission
			m.permissions.Toggle(m.cursor)
		case "n":
			m.permissions.Network = !m.permissions.Network
		case "a":
			m.permissions.Audio = !m.permissions.Audio
		case "g":
			m.permissions.GPU = !m.permissions.GPU
		case "w":
			m.permissions.Wayland = !m.permissions.Wayland
		case "x":
			m.permissions.X11 = !m.permissions.X11
		case "c":
			m.permissions.Camera = !m.permissions.Camera
		case "p":
			m.permissions.Portals = !m.permissions.Portals
		}
	}
	return m, nil
}

func (m model) updateAppSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.cursor = 0
			m.state = viewBottleActions
			return m, nil
		case "enter":
			if i, ok := m.appList.SelectedItem().(appItem); ok {
				m.selectedApp = i.app
				m.permissions.LastApp = i.app.ID
				savePermissions(m.configPath, m.permissions)
				m.state = viewLaunchConfirm
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	m.appList, cmd = m.appList.Update(msg)
	return m, cmd
}

func (m model) updateLaunchConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.state = viewAppSelect
			return m, nil
		case "enter", "l", "1":
			// Launch - check if already mounted
			loopDev := findLoopForFile(m.selectedBottle)
			if loopDev != "" {
				cleartext := findCleartextForLoop(loopDev)
				if cleartext != "" {
					mount := findMountForDevice(cleartext)
					if mount != "" {
						// Already mounted, just run
						m.mountInfo = &MountInfo{
							LoopDevice:      loopDev,
							CleartextDevice: cleartext,
							MountPoint:      mount,
						}
						SetCurrentMountInfo(m.mountInfo) // Update global for signal handler
						m.state = viewRunning
						return m, runFlatpakCmd(m.selectedApp.ID, mount, m.permissions, nil)
					}
				}
			}

			// Check if this is a FIDO2 bottle
			isFIDO2, err := IsFIDO2Bottle(m.permissions)
			if err != nil {
				// Corrupted config
				m.errMsg = err.Error()
				m.state = viewError
				return m, nil
			}

			if isFIDO2 {
				// FIDO2 bottle - go to YubiKey unlock
				m.bottleUsesYubiKey = true
				m.fido2Error = ""
				m.fido2Devices = nil
				m.state = viewFIDO2Unlock
				m.loading = true
				m.loadingMsg = "Looking for YubiKey..."
				return m, enumerateFIDO2DevicesCmd()
			}

			// Password bottle
			m.passwordInput.Reset()
			m.passwordInput.Focus()
			m.state = viewPasswordInput
			return m, textinput.Blink
		case "p", "2":
			// Edit permissions first
			m.cursor = 0
			m.prevState = viewLaunchConfirm
			m.state = viewPermissions
			return m, nil
		}
	}
	return m, nil
}

func (m model) updatePasswordInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.state = viewLaunchConfirm
			return m, nil
		case "enter":
			m.password = m.passwordInput.Value()
			if m.password == "" {
				return m, nil
			}
			m.loading = true
			m.loadingMsg = "Unlocking bottle..."
			return m, mountBottleCmd(m.selectedBottle, m.password)
		}
	}

	var cmd tea.Cmd
	m.passwordInput, cmd = m.passwordInput.Update(msg)
	return m, cmd
}

func (m model) updateCreateBottle(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.createForm.State == huh.StateNormal {
				m.state = viewBottleList
				return m, nil
			}
		}
	}

	form, cmd := m.createForm.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.createForm = f

		if m.createForm.State == huh.StateCompleted {
			// Extract form values
			name := m.createForm.GetString("name")
			size := m.createForm.GetString("size")
			password := m.createForm.GetString("password")
			confirm := m.createForm.GetString("confirm")

			// Validate password confirmation
			if password != confirm {
				m.errMsg = "Passwords do not match"
				m.state = viewError
				return m, nil
			}

			if name != "" && size != "" && password != "" {
				m.loading = true
				m.loadingMsg = "Creating bottle..."
				return m, createBottleCmd(name, size, password)
			}
			m.state = viewBottleList
			return m, nil
		}
	}

	return m, cmd
}

func (m model) updateDeleteConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "n":
			m.cursor = 0
			m.state = viewBottleActions
			return m, nil
		case "y", "enter":
			// Check if mounted
			loopDev := findLoopForFile(m.selectedBottle)
			if loopDev != "" {
				m.errMsg = "Bottle is currently mounted. Close any running apps first."
				m.state = viewError
				return m, nil
			}
			m.loading = true
			m.loadingMsg = "Deleting bottle..."
			return m, deleteBottleCmd(m.selectedBottle)
		}
	}
	return m, nil
}

func (m model) updateError(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "enter":
			m.err = nil
			m.errMsg = ""
			m.state = viewBottleList
			return m, nil
		}
	}
	return m, nil
}

func (m model) updateCreateBottleYubiKey(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			// Handle escape based on current step
			if m.fido2Step == 0 && m.createForm != nil && m.createForm.State == huh.StateNormal {
				m.state = viewBottleList
				return m, nil
			} else if m.fido2Step == 4 {
				// Success step - go back to bottle list
				m.state = viewBottleList
				return m, loadBottlesCmd()
			} else if m.fido2Step > 0 {
				// Cancel creation in progress
				m.fido2Secret = nil
				m.state = viewBottleList
				return m, nil
			}
		case "enter":
			// Handle enter based on step
			switch m.fido2Step {
			case 1:
				// Device selected, start credential creation
				if len(m.fido2Devices) > 0 {
					m.loading = true
					m.loadingMsg = "Touch YubiKey to create credential..."
					device := m.fido2Devices[m.fido2DeviceSel].Path
					return m, createFIDO2CredentialCmd(device, m.fido2BottleID)
				}
			case 2:
				// Credential created, get secret
				m.loading = true
				m.loadingMsg = "Touch YubiKey to generate encryption key..."
				device := m.fido2Devices[m.fido2DeviceSel].Path
				return m, getFIDO2SecretCmd(device, m.fido2BottleID, m.fido2CredID, m.fido2Salt)
			case 3:
				// Secret ready, create bottle
				m.loading = true
				m.loadingMsg = "Creating encrypted bottle..."
				device := m.fido2Devices[m.fido2DeviceSel].Path
				return m, createBottleYubiKeyCmd(
					m.fido2BottleName,
					m.fido2BottleSize,
					m.fido2Secret,
					m.fido2BottleID,
					m.fido2CredID,
					m.fido2Salt,
					device,
				)
			case 4:
				// Success, go back to bottle list
				m.state = viewBottleList
				return m, loadBottlesCmd()
			}
		case "r":
			// Retry device enumeration
			if m.fido2Step == 1 && len(m.fido2Devices) == 0 {
				m.loading = true
				m.loadingMsg = "Looking for YubiKey..."
				return m, enumerateFIDO2DevicesCmd()
			}
		case "up", "k":
			if m.fido2Step == 1 && m.fido2DeviceSel > 0 {
				m.fido2DeviceSel--
			}
		case "down", "j":
			if m.fido2Step == 1 && m.fido2DeviceSel < len(m.fido2Devices)-1 {
				m.fido2DeviceSel++
			}
		}
	}

	// Handle form update in step 0
	if m.fido2Step == 0 && m.createForm != nil {
		form, cmd := m.createForm.Update(msg)
		if f, ok := form.(*huh.Form); ok {
			m.createForm = f

			if m.createForm.State == huh.StateCompleted {
				// Extract form values
				name := m.createForm.GetString("name")
				size := m.createForm.GetString("size")

				if name != "" && size != "" {
					m.fido2BottleName = name
					m.fido2BottleSize = size

					// Check prerequisites
					if err := CheckFIDO2Available(); err != nil {
						m.fido2Error = err.Error()
						m.fido2Step = -1 // Error step
						return m, nil
					}
					if err := CheckPrivilegeEscalation(); err != nil {
						m.fido2Error = err.Error()
						m.fido2Step = -1
						return m, nil
					}
					if err := CheckUdisksAvailable(); err != nil {
						m.fido2Error = err.Error()
						m.fido2Step = -1
						return m, nil
					}

					// Generate bottle ID
					bottleID, err := generateBottleID()
					if err != nil {
						m.fido2Error = err.Error()
						m.fido2Step = -1
						return m, nil
					}
					m.fido2BottleID = bottleID

					// Move to device enumeration
					m.fido2Step = 1
					m.loading = true
					m.loadingMsg = "Looking for YubiKey..."
					return m, enumerateFIDO2DevicesCmd()
				}
				m.state = viewBottleList
				return m, nil
			}
		}
		return m, cmd
	}

	return m, nil
}

func (m model) updateFIDO2Unlock(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.fido2Secret = nil
			m.fido2Error = ""
			m.state = viewLaunchConfirm
			return m, nil
		case "r":
			// Retry
			m.fido2Error = ""
			m.loading = true
			m.loadingMsg = "Looking for YubiKey..."
			return m, enumerateFIDO2DevicesCmd()
		case "enter":
			// Try to unlock if we have devices
			if len(m.fido2Devices) > 0 {
				m.loading = true
				m.loadingMsg = "Touch YubiKey to unlock..."
				device := m.fido2Devices[m.fido2DeviceSel].Path
				return m, mountBottleFIDO2Cmd(
					m.selectedBottle,
					device,
					m.permissions.FIDO2BottleID,
					m.permissions.FIDO2CredentialID,
					m.permissions.FIDO2Salt,
				)
			}
		case "up", "k":
			if m.fido2DeviceSel > 0 {
				m.fido2DeviceSel--
			}
		case "down", "j":
			if m.fido2DeviceSel < len(m.fido2Devices)-1 {
				m.fido2DeviceSel++
			}
		}
	}

	// Auto-unlock if devices were just enumerated and there's exactly one
	if len(m.fido2Devices) == 1 && m.fido2Error == "" && !m.loading {
		m.loading = true
		m.loadingMsg = "Touch YubiKey to unlock..."
		device := m.fido2Devices[0].Path
		return m, mountBottleFIDO2Cmd(
			m.selectedBottle,
			device,
			m.permissions.FIDO2BottleID,
			m.permissions.FIDO2CredentialID,
			m.permissions.FIDO2Salt,
		)
	}

	return m, nil
}

func (m model) View() string {
	if m.loading {
		return m.renderLoading()
	}

	var content string
	switch m.state {
	case viewBottleList:
		content = m.renderBottleList()
	case viewBottleActions:
		content = m.renderBottleActions()
	case viewPermissions:
		content = m.renderPermissions()
	case viewAppSelect:
		content = m.renderAppSelect()
	case viewLaunchConfirm:
		content = m.renderLaunchConfirm()
	case viewPasswordInput:
		content = m.renderPasswordInput()
	case viewCreateBottle:
		content = m.renderCreateBottle()
	case viewDeleteConfirm:
		content = m.renderDeleteConfirm()
	case viewRunning:
		content = m.renderRunning()
	case viewError:
		content = m.renderError()
	case viewCreateBottleYubiKey:
		content = m.renderCreateBottleYubiKey()
	case viewFIDO2Unlock:
		content = m.renderFIDO2Unlock()
	default:
		content = "Unknown state"
	}

	return content
}
