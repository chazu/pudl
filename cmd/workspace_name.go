package cmd

// effectiveWorkspaceName is where this invocation stands: the repo workspace's
// name, or "global" outside one.
//
// Recorded on the durable rows a run writes (snapshot provenance), rather than
// resolved when they are read back. It is history: reading it from the current
// context would answer where the *reader* is standing, and would silently
// relabel every past snapshot when a repo moves or is renamed.
func effectiveWorkspaceName() string {
	if wsPolicy == nil {
		return "global"
	}
	return wsPolicy.EffectiveOrigin
}
