package pulumi

// A Pulumi Cloud stack and its latest update: one record per stack, from
// GET /api/stacks/<org>/<project>/<stack>/updates?pageSize=1&page=1. The
// collector's transform keeps only these fields. It drops the update's
// message, config, deployment and resource changes, and every environment
// key except vcs.kind, vcs.owner, vcs.repo, vcs.root, git.head,
// git.headName, git.dirty and pulumi.version. A stack with no updates has
// no record.
#Stack: {
	_pudl: {
		schema_type:   "base"
		resource_type: "pulumi.stack"
		identity_fields: ["stack"]
		review: status: "draft"
		// Arguments the update did not record are left out of the fact.
		facts: pulumi_stack_source: args: {
			stack:  "stack"
			repo:   "source.repo"
			root:   "source.root"
			commit: "source.commit"
			dirty:  "source.dirty"
		}
	}
	// <org>/<project>/<stack>
	stack: =~"^[^/]+/[^/]+/[^/]+$"
	// The update's sequence number in the stack's history.
	version: int
	// update, refresh, destroy, import, ...
	kind:   string
	result: string
	// Unix seconds.
	startTime:      int
	endTime?:       int
	resourceCount?: int
	pulumiVersion?: string
	// Where the update ran from, as the CLI recorded it.
	source: {
		// <owner>/<repo>
		repo?: string
		// The VCS host, e.g. github.com.
		vcs?: string
		// The directory of the Pulumi project in the repo.
		root?: string
		// The commit checked out (git.head), approximate when dirty.
		commit?: string
		branch?: string
		// The working tree had uncommitted changes.
		dirty: bool
	}
}
