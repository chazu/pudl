package fileset

import "os"

// State is how a workspace's install record of a file set compares with the
// file set this pudl ships.
type State string

const (
	// NotInstalled: the workspace has no install record of the file set.
	NotInstalled State = "not-installed"
	// Installed: the last install wrote exactly this version's files.
	Installed State = "installed"
	// Outdated: the last install wrote another version; installing again upgrades it.
	Outdated State = "outdated"
)

// RecordedState compares the workspace's install record of file set name with
// files. It reads only the record, not the installed files, so it does not
// notice edits made since the install.
func RecordedState(root string, kind Kind, name string, files map[string][]byte) (State, error) {
	if _, err := os.Stat(manifestPath(root, kind, name)); os.IsNotExist(err) {
		return NotInstalled, nil
	} else if err != nil {
		return "", err
	}
	recorded, err := readManifest(root, kind, name)
	if err != nil {
		return "", err
	}
	if len(recorded) != len(files) {
		return Outdated, nil
	}
	for path, data := range files {
		if recorded[path] != digest(data) {
			return Outdated, nil
		}
	}
	return Installed, nil
}
