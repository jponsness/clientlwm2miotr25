// -*- Mode: Go; indent-tabs-mode: t -*-

/*
 * Copyright (C) 2014-2016 Canonical Ltd
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License version 3 as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

package snap

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/snapcore/snapd/dirs"
	"github.com/snapcore/snapd/strutil"
	"github.com/snapcore/snapd/timeout"
)

// PlaceInfo offers all the information about where a snap and its data are located and exposed in the filesystem.
type PlaceInfo interface {
	// Name returns the name of the snap.
	Name() string

	// MountDir returns the base directory of the snap.
	MountDir() string

	// MountFile returns the path where the snap file that is mounted is installed.
	MountFile() string

	// HooksDir returns the directory containing the snap's hooks.
	HooksDir() string

	// DataDir returns the data directory of the snap.
	DataDir() string

	// UserDataDir returns the per user data directory of the snap.
	UserDataDir(home string) string

	// CommonDataDir returns the data directory common across revisions of the snap.
	CommonDataDir() string

	// UserCommonDataDir returns the per user data directory common across revisions of the snap.
	UserCommonDataDir(home string) string

	// UserXdgRuntimeDir returns the per user XDG_RUNTIME_DIR directory
	UserXdgRuntimeDir(userID int) string

	// DataHomeDir returns the a glob that matches all per user data directories of a snap.
	DataHomeDir() string

	// CommonDataHomeDir returns a glob that matches all per user data directories common across revisions of the snap.
	CommonDataHomeDir() string

	// XdgRuntimeDirs returns a glob that matches all XDG_RUNTIME_DIR directories for all users of the snap.
	XdgRuntimeDirs() string
}

// MinimalPlaceInfo returns a PlaceInfo with just the location information for a snap of the given name and revision.
func MinimalPlaceInfo(name string, revision Revision) PlaceInfo {
	return &Info{SideInfo: SideInfo{RealName: name, Revision: revision}}
}

// MountDir returns the base directory where it gets mounted of the snap with the given name and revision.
func MountDir(name string, revision Revision) string {
	return filepath.Join(dirs.SnapMountDir, name, revision.String())
}

// MountFile returns the path where the snap file that is mounted is installed.
func MountFile(name string, revision Revision) string {
	return filepath.Join(dirs.SnapBlobDir, fmt.Sprintf("%s_%s.snap", name, revision))
}

// ScopedSecurityTag returns the snap-specific, scope specific, security tag.
func ScopedSecurityTag(snapName, scopeName, suffix string) string {
	return fmt.Sprintf("snap.%s.%s.%s", snapName, scopeName, suffix)
}

// SecurityTag returns the snap-specific security tag.
func SecurityTag(snapName string) string {
	return fmt.Sprintf("snap.%s", snapName)
}

// AppSecurityTag returns the application-specific security tag.
func AppSecurityTag(snapName, appName string) string {
	return fmt.Sprintf("%s.%s", SecurityTag(snapName), appName)
}

// HookSecurityTag returns the hook-specific security tag.
func HookSecurityTag(snapName, hookName string) string {
	return ScopedSecurityTag(snapName, "hook", hookName)
}

// NoneSecurityTag returns the security tag for interfaces that
// are not associated to an app or hook in the snap.
func NoneSecurityTag(snapName, uniqueName string) string {
	return ScopedSecurityTag(snapName, "none", uniqueName)
}

// SideInfo holds snap metadata that is crucial for the tracking of
// snaps and for the working of the system offline and which is not
// included in snap.yaml or for which the store is the canonical
// source overriding snap.yaml content.
//
// It can be marshalled and will be stored in the system state for
// each currently installed snap revision so it needs to be evolved
// carefully.
//
// Information that can be taken directly from snap.yaml or that comes
// from the store but is not required for working offline should not
// end up in SideInfo.
type SideInfo struct {
	RealName          string   `yaml:"name,omitempty" json:"name,omitempty"`
	SnapID            string   `yaml:"snap-id" json:"snap-id"`
	Revision          Revision `yaml:"revision" json:"revision"`
	Channel           string   `yaml:"channel,omitempty" json:"channel,omitempty"`
	Contact           string   `yaml:"contact,omitempty" json:"contact,omitempty"`
	EditedTitle       string   `yaml:"title,omitempty" json:"title,omitempty"`
	EditedSummary     string   `yaml:"summary,omitempty" json:"summary,omitempty"`
	EditedDescription string   `yaml:"description,omitempty" json:"description,omitempty"`
	Private           bool     `yaml:"private,omitempty" json:"private,omitempty"`
}

// Info provides information about snaps.
type Info struct {
	SuggestedName string
	Version       string
	Type          Type
	Architectures []string
	Assumes       []string

	OriginalTitle       string
	OriginalSummary     string
	OriginalDescription string

	Environment strutil.OrderedMap

	LicenseAgreement string
	LicenseVersion   string
	License          string
	Epoch            Epoch
	Base             string
	Confinement      ConfinementType
	Apps             map[string]*AppInfo
	LegacyAliases    map[string]*AppInfo // FIXME: eventually drop this
	Hooks            map[string]*HookInfo
	Plugs            map[string]*PlugInfo
	Slots            map[string]*SlotInfo

	// Plugs or slots with issues (they are not included in Plugs or Slots)
	BadInterfaces map[string]string // slot or plug => message

	// The information in all the remaining fields is not sourced from the snap blob itself.
	SideInfo

	// Broken marks if set whether the snap is broken and the reason.
	Broken string

	// The information in these fields is ephemeral, available only from the store.
	DownloadInfo

	IconURL string
	Prices  map[string]float64
	MustBuy bool

	PublisherID string
	Publisher   string

	Screenshots []ScreenshotInfo

	// The flattended channel map with $track/$risk
	Channels map[string]*ChannelSnapInfo

	// The ordered list of tracks that contain channels
	Tracks []string

	Layout map[string]*Layout
}

// Layout describes a single element of the layout section.
type Layout struct {
	Snap *Info

	Path    string      `json:"path"`
	Bind    string      `json:"bind,omitempty"`
	Type    string      `json:"type,omitempty"`
	User    string      `json:"user,omitempty"`
	Group   string      `json:"group,omitempty"`
	Mode    os.FileMode `json:"mode,omitempty"`
	Symlink string      `json:"symlink,omitempty"`
}

// ChannelSnapInfo is the minimum information that can be used to clearly
// distinguish different revisions of the same snap.
type ChannelSnapInfo struct {
	Revision    Revision        `json:"revision"`
	Confinement ConfinementType `json:"confinement"`
	Version     string          `json:"version"`
	Channel     string          `json:"channel"`
	Epoch       Epoch           `json:"epoch"`
	Size        int64           `json:"size"`
}

// Name returns the blessed name for the snap.
func (s *Info) Name() string {
	if s.RealName != "" {
		return s.RealName
	}
	return s.SuggestedName
}

// Title returns the blessed title for the snap.
func (s *Info) Title() string {
	if s.EditedTitle != "" {
		return s.EditedTitle
	}
	return s.OriginalTitle
}

// Summary returns the blessed summary for the snap.
func (s *Info) Summary() string {
	if s.EditedSummary != "" {
		return s.EditedSummary
	}
	return s.OriginalSummary
}

// Description returns the blessed description for the snap.
func (s *Info) Description() string {
	if s.EditedDescription != "" {
		return s.EditedDescription
	}
	return s.OriginalDescription
}

// MountDir returns the base directory of the snap where it gets mounted.
func (s *Info) MountDir() string {
	return MountDir(s.Name(), s.Revision)
}

// MountFile returns the path where the snap file that is mounted is installed.
func (s *Info) MountFile() string {
	return MountFile(s.Name(), s.Revision)
}

// HooksDir returns the directory containing the snap's hooks.
func (s *Info) HooksDir() string {
	return filepath.Join(s.MountDir(), "meta", "hooks")
}

// DataDir returns the data directory of the snap.
func (s *Info) DataDir() string {
	return filepath.Join(dirs.SnapDataDir, s.Name(), s.Revision.String())
}

// UserDataDir returns the user-specific data directory of the snap.
func (s *Info) UserDataDir(home string) string {
	return filepath.Join(home, "snap", s.Name(), s.Revision.String())
}

// UserCommonDataDir returns the user-specific data directory common across revision of the snap.
func (s *Info) UserCommonDataDir(home string) string {
	return filepath.Join(home, "snap", s.Name(), "common")
}

// CommonDataDir returns the data directory common across revisions of the snap.
func (s *Info) CommonDataDir() string {
	return filepath.Join(dirs.SnapDataDir, s.Name(), "common")
}

// DataHomeDir returns the per user data directory of the snap.
func (s *Info) DataHomeDir() string {
	return filepath.Join(dirs.SnapDataHomeGlob, s.Name(), s.Revision.String())
}

// CommonDataHomeDir returns the per user data directory common across revisions of the snap.
func (s *Info) CommonDataHomeDir() string {
	return filepath.Join(dirs.SnapDataHomeGlob, s.Name(), "common")
}

// UserXdgRuntimeDir returns the XDG_RUNTIME_DIR directory of the snap for a particular user.
func (s *Info) UserXdgRuntimeDir(euid int) string {
	return filepath.Join("/run/user", fmt.Sprintf("%d/snap.%s", euid, s.Name()))
}

// XdgRuntimeDirs returns the XDG_RUNTIME_DIR directories for all users of the snap.
func (s *Info) XdgRuntimeDirs() string {
	return filepath.Join(dirs.XdgRuntimeDirGlob, fmt.Sprintf("snap.%s", s.Name()))
}

// NeedsDevMode returns whether the snap needs devmode.
func (s *Info) NeedsDevMode() bool {
	return s.Confinement == DevModeConfinement
}

// NeedsClassic  returns whether the snap needs classic confinement consent.
func (s *Info) NeedsClassic() bool {
	return s.Confinement == ClassicConfinement
}

// Services returns a list of the apps that have "daemon" set.
func (s *Info) Services() []*AppInfo {
	svcs := make([]*AppInfo, 0, len(s.Apps))
	for _, app := range s.Apps {
		if !app.IsService() {
			continue
		}
		svcs = append(svcs, app)
	}

	return svcs
}

func BadInterfacesSummary(snapInfo *Info) string {
	inverted := make(map[string][]string)
	for name, reason := range snapInfo.BadInterfaces {
		inverted[reason] = append(inverted[reason], name)
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "snap %q has bad plugs or slots: ", snapInfo.Name())
	reasons := make([]string, 0, len(inverted))
	for reason := range inverted {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	for _, reason := range reasons {
		names := inverted[reason]
		sort.Strings(names)
		for i, name := range names {
			if i > 0 {
				buf.WriteString(", ")
			}
			buf.WriteString(name)
		}
		fmt.Fprintf(&buf, " (%s); ", reason)
	}
	return strings.TrimSuffix(buf.String(), "; ")
}

// DownloadInfo contains the information to download a snap.
// It can be marshalled.
type DownloadInfo struct {
	AnonDownloadURL string `json:"anon-download-url,omitempty"`
	DownloadURL     string `json:"download-url,omitempty"`

	Size     int64  `json:"size,omitempty"`
	Sha3_384 string `json:"sha3-384,omitempty"`

	// The server can include information about available deltas for a given
	// snap at a specific revision during refresh. Currently during refresh the
	// server will provide single matching deltas only, from the clients
	// revision to the target revision when available, per requested format.
	Deltas []DeltaInfo `json:"deltas,omitempty"`
}

// DeltaInfo contains the information to download a delta
// from one revision to another.
type DeltaInfo struct {
	FromRevision    int    `json:"from-revision,omitempty"`
	ToRevision      int    `json:"to-revision,omitempty"`
	Format          string `json:"format,omitempty"`
	AnonDownloadURL string `json:"anon-download-url,omitempty"`
	DownloadURL     string `json:"download-url,omitempty"`
	Size            int64  `json:"size,omitempty"`
	Sha3_384        string `json:"sha3-384,omitempty"`
}

// sanity check that Info is a PlaceInfo
var _ PlaceInfo = (*Info)(nil)

// PlugInfo provides information about a plug.
type PlugInfo struct {
	Snap *Info

	Name      string
	Interface string
	Attrs     map[string]interface{}
	Label     string
	Apps      map[string]*AppInfo
	Hooks     map[string]*HookInfo
}

// SecurityTags returns security tags associated with a given plug.
func (plug *PlugInfo) SecurityTags() []string {
	tags := make([]string, 0, len(plug.Apps)+len(plug.Hooks))
	for _, app := range plug.Apps {
		tags = append(tags, app.SecurityTag())
	}
	for _, hook := range plug.Hooks {
		tags = append(tags, hook.SecurityTag())
	}
	sort.Strings(tags)
	return tags
}

// String returns the representation of the plug as snap:plug string.
func (plug *PlugInfo) String() string {
	return fmt.Sprintf("%s:%s", plug.Snap.Name(), plug.Name)
}

// SecurityTags returns security tags associated with a given slot.
func (slot *SlotInfo) SecurityTags() []string {
	tags := make([]string, 0, len(slot.Apps))
	for _, app := range slot.Apps {
		tags = append(tags, app.SecurityTag())
	}
	for _, hook := range slot.Hooks {
		tags = append(tags, hook.SecurityTag())
	}
	sort.Strings(tags)
	return tags
}

// String returns the representation of the slot as snap:slot string.
func (slot *SlotInfo) String() string {
	return fmt.Sprintf("%s:%s", slot.Snap.Name(), slot.Name)
}

// SlotInfo provides information about a slot.
type SlotInfo struct {
	Snap *Info

	Name      string
	Interface string
	Attrs     map[string]interface{}
	Label     string
	Apps      map[string]*AppInfo
	Hooks     map[string]*HookInfo
}

// SocketInfo provides information on application sockets.
type SocketInfo struct {
	App *AppInfo

	Name         string
	ListenStream string
	SocketMode   os.FileMode
}

// AppInfo provides information about a app.
type AppInfo struct {
	Snap *Info

	Name          string
	LegacyAliases []string // FIXME: eventually drop this
	Command       string

	Daemon          string
	StopTimeout     timeout.Timeout
	StopCommand     string
	ReloadCommand   string
	PostStopCommand string
	RestartCond     RestartCondition
	Completer       string

	// TODO: this should go away once we have more plumbing and can change
	// things vs refactor
	// https://github.com/snapcore/snapd/pull/794#discussion_r58688496
	BusName string

	Plugs   map[string]*PlugInfo
	Slots   map[string]*SlotInfo
	Sockets map[string]*SocketInfo

	Environment strutil.OrderedMap
}

// ScreenshotInfo provides information about a screenshot.
type ScreenshotInfo struct {
	URL    string
	Width  int64
	Height int64
}

// HookInfo provides information about a hook.
type HookInfo struct {
	Snap *Info

	Name  string
	Plugs map[string]*PlugInfo
	Slots map[string]*SlotInfo
}

// File returns the path to the file
func (socket *SocketInfo) File() string {
	return filepath.Join(dirs.SnapServicesDir, socket.App.SecurityTag()+"."+socket.Name+".socket")
}

// SecurityTag returns application-specific security tag.
//
// Security tags are used by various security subsystems as "profile names" and
// sometimes also as a part of the file name.
func (app *AppInfo) SecurityTag() string {
	return AppSecurityTag(app.Snap.Name(), app.Name)
}

func (app *AppInfo) DesktopFile() string {
	return filepath.Join(dirs.SnapDesktopFilesDir, fmt.Sprintf("%s_%s.desktop", app.Snap.Name(), app.Name))
}

// WrapperPath returns the path to wrapper invoking the app binary.
func (app *AppInfo) WrapperPath() string {
	return filepath.Join(dirs.SnapBinariesDir, JoinSnapApp(app.Snap.Name(), app.Name))
}

// CompleterPath returns the path to the completer snippet for the app binary.
func (app *AppInfo) CompleterPath() string {
	return filepath.Join(dirs.CompletersDir, JoinSnapApp(app.Snap.Name(), app.Name))
}

func (app *AppInfo) launcherCommand(command string) string {
	if command != "" {
		command = " " + command
	}
	if app.Name == app.Snap.Name() {
		return fmt.Sprintf("/usr/bin/snap run%s %s", command, app.Name)
	}
	return fmt.Sprintf("/usr/bin/snap run%s %s.%s", command, app.Snap.Name(), app.Name)
}

// LauncherCommand returns the launcher command line to use when invoking the app binary.
func (app *AppInfo) LauncherCommand() string {
	return app.launcherCommand("")
}

// LauncherStopCommand returns the launcher command line to use when invoking the app stop command binary.
func (app *AppInfo) LauncherStopCommand() string {
	return app.launcherCommand("--command=stop")
}

// LauncherReloadCommand returns the launcher command line to use when invoking the app stop command binary.
func (app *AppInfo) LauncherReloadCommand() string {
	return app.launcherCommand("--command=reload")
}

// LauncherPostStopCommand returns the launcher command line to use when invoking the app post-stop command binary.
func (app *AppInfo) LauncherPostStopCommand() string {
	return app.launcherCommand("--command=post-stop")
}

// ServiceName returns the systemd service name for the daemon app.
func (app *AppInfo) ServiceName() string {
	return app.SecurityTag() + ".service"
}

// ServiceFile returns the systemd service file path for the daemon app.
func (app *AppInfo) ServiceFile() string {
	return filepath.Join(dirs.SnapServicesDir, app.ServiceName())
}

// Env returns the app specific environment overrides
func (app *AppInfo) Env() []string {
	env := []string{}
	appEnv := app.Snap.Environment.Copy()
	for _, k := range app.Environment.Keys() {
		appEnv.Set(k, app.Environment.Get(k))
	}
	for _, k := range appEnv.Keys() {
		env = append(env, fmt.Sprintf("%s=%s", k, appEnv.Get(k)))
	}
	return env
}

// IsService returns whether app represents a daemon/service.
func (app *AppInfo) IsService() bool {
	return app.Daemon != ""
}

// SecurityTag returns the hook-specific security tag.
//
// Security tags are used by various security subsystems as "profile names" and
// sometimes also as a part of the file name.
func (hook *HookInfo) SecurityTag() string {
	return HookSecurityTag(hook.Snap.Name(), hook.Name)
}

// Env returns the hook-specific environment overrides
func (hook *HookInfo) Env() []string {
	env := []string{}
	hookEnv := hook.Snap.Environment.Copy()
	for _, k := range hookEnv.Keys() {
		env = append(env, fmt.Sprintf("%s=%s\n", k, hookEnv.Get(k)))
	}
	return env
}

func infoFromSnapYamlWithSideInfo(meta []byte, si *SideInfo) (*Info, error) {
	info, err := InfoFromSnapYaml(meta)
	if err != nil {
		return nil, err
	}

	if si != nil {
		info.SideInfo = *si
	}

	return info, nil
}

type NotFoundError struct {
	Snap     string
	Revision Revision
}

func (e NotFoundError) Error() string {
	return fmt.Sprintf("cannot find installed snap %q at revision %s", e.Snap, e.Revision)
}

func MockSanitizePlugsSlots(f func(snapInfo *Info)) (restore func()) {
	old := SanitizePlugsSlots
	SanitizePlugsSlots = f
	return func() { SanitizePlugsSlots = old }
}

var SanitizePlugsSlots = func(snapInfo *Info) {
	panic("SanitizePlugsSlots function not set")
}

// ReadInfo reads the snap information for the installed snap with the given name and given side-info.
func ReadInfo(name string, si *SideInfo) (*Info, error) {
	snapYamlFn := filepath.Join(MountDir(name, si.Revision), "meta", "snap.yaml")
	meta, err := ioutil.ReadFile(snapYamlFn)
	if os.IsNotExist(err) {
		return nil, &NotFoundError{Snap: name, Revision: si.Revision}
	}
	if err != nil {
		return nil, err
	}

	info, err := infoFromSnapYamlWithSideInfo(meta, si)
	if err != nil {
		return nil, err
	}

	st, err := os.Stat(MountFile(name, si.Revision))
	if err != nil {
		return nil, err
	}
	info.Size = st.Size()

	err = addImplicitHooks(info)
	if err != nil {
		return nil, err
	}

	SanitizePlugsSlots(info)

	return info, nil
}

// ReadInfoFromSnapFile reads the snap information from the given File
// and completes it with the given side-info if this is not nil.
func ReadInfoFromSnapFile(snapf Container, si *SideInfo) (*Info, error) {
	meta, err := snapf.ReadFile("meta/snap.yaml")
	if err != nil {
		return nil, err
	}

	info, err := infoFromSnapYamlWithSideInfo(meta, si)
	if err != nil {
		return nil, err
	}

	info.Size, err = snapf.Size()
	if err != nil {
		return nil, err
	}

	err = addImplicitHooksFromContainer(info, snapf)
	if err != nil {
		return nil, err
	}

	err = Validate(info)
	if err != nil {
		return nil, err
	}

	return info, nil
}

// SplitSnapApp will split a string of the form `snap.app` into
// the `snap` and the `app` part. It also deals with the special
// case of snapName == appName.
func SplitSnapApp(snapApp string) (snap, app string) {
	l := strings.SplitN(snapApp, ".", 2)
	if len(l) < 2 {
		return l[0], l[0]
	}
	return l[0], l[1]
}

// JoinSnapApp produces a full application wrapper name from the
// `snap` and the `app` part. It also deals with the special
// case of snapName == appName.
func JoinSnapApp(snap, app string) string {
	if snap == app {
		return app
	}
	return fmt.Sprintf("%s.%s", snap, app)
}
