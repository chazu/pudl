package git

// GitRemote is a named remote of a repository. It is a *component* (no `_pudl`
// block): a reusable shape embedded inline in #GitRepository, not a tracked
// resource of its own. Re-importing a repository replaces its whole `remotes`
// array, so removed remotes drop implicitly.
#GitRemote: {
	name:      string // "origin"
	url:       string
	push_url?: string
	...
}

// GitBranch is a branch of a repository. Like #GitRemote it is an inline
// *component*, not a separate tracked resource: this swing does not need
// per-branch bitemporal history (the branch tip is captured as a point-in-time
// value on the repository). If independent per-branch history is ever required,
// #GitBranch would be promoted to its own `_pudl` resource -- see
// docs/issues/git-repository-decomposed-resources.md (D3, C1-C4).
#GitBranch: {
	name: string // "main", "release/1.2"
	sha:  string // current tip
	...
}

// GitRepository is the platform-agnostic root of the git repository family.
//
// Identity is `name` only: the fully-qualified path (e.g. "github.com/owner/repo"
// or a filesystem path for a local clone), which git itself does not assign, so
// it must be supplied as a globally-unique handle. `root_commit` is attractive as
// identity but is optional (empty repos have none, a repo may have several), and
// an identity field that is sometimes absent would split one logical resource in
// two -- so it is tracked, not identity. See the design doc (D2).
#GitRepository: {
	_pudl: {
		schema_type:     "base"
		resource_type:   string | *"git.repository"
		identity_fields: ["name"]
		tracked_fields:  ["default_branch", "root_commit"]
		// Declared (optional) so platform specializations built with
		// `#Child: #GitRepository & {...}` may set it; `_pudl` is closed
		// inside a definition, so an undeclared field would be rejected.
		base_schema?: string
	}

	name:           string // fully-qualified path, globally unique
	default_branch: string
	bare?:          bool
	root_commit?:   string // first (parentless) commit; optional => tracked, not identity
	remotes: [...#GitRemote]
	branches: [...#GitBranch]
	...
}

// GitHubRepository specializes the family to repositories hosted on github.com.
// It only tightens the `name` constraint (per the family-identity invariant,
// identity_fields stay ["name"], inherited unchanged) and adds optional
// platform fields; `resource_type` narrows to "git.repository.github" so the
// origin keyword "github" helps inference disambiguate it.
#GitHubRepository: #GitRepository & {
	_pudl: {
		resource_type: "git.repository.github"
		base_schema:   "pudl/git.#GitRepository"
	}

	name:        =~"^github\\.com/" // "github.com/owner/repo"
	owner?:      string
	visibility?: "public" | "private" | "internal"
}

// GitLabRepository specializes the family to repositories hosted on gitlab.com.
// Same shape rules as #GitHubRepository; self-hosted GitLab instances use other
// hosts and are left to user-authored variants.
#GitLabRepository: #GitRepository & {
	_pudl: {
		resource_type: "git.repository.gitlab"
		base_schema:   "pudl/git.#GitRepository"
	}

	name:        =~"^gitlab\\.com/" // "gitlab.com/group/project"
	namespace?:  string
	visibility?: "public" | "private" | "internal"
}
