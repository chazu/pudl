package models

import sm "pudl.schemas/pudl/systemmodel@v0"

#GitInventory: sm.#SystemModel & {
	name: "git-inventory"
	plugins: [{
		name: "git-inventory"
		// mu resolves command paths from the generated package, one directory
		// beneath .pudl/data/mu. This observer needs no global plugin install.
		command: ["python3", "../../../populators/git-inventory/observe.py"]
	}]
	populate: {
		plugin:       "git-inventory"
		differential: false
		// Relative to the mu root at .pudl/data/mu.
		input: inventory: "../../populators/git-inventory/current.json"
	}
	desired: [{
		"_schema":     "git.repository"
		name:           "local/demo"
		default_branch: "main"
	}]
}
