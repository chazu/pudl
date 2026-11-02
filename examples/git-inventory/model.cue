package models

import sm "pudl.schemas/pudl/systemmodel@v0"

#GitInventory: sm.#SystemModel & {
	name: "git-inventory"
	populate: {
		// The walkthrough's supplied inventory stands in for a real Git
		// observer: any command that prints JSON records works here. The path
		// is relative to this model file.
		runs: [{argv: ["cat", "../../populators/git-inventory/current.json"]}]
	}
	desired: [{
		"_schema":     "git.repository"
		name:           "local/demo"
		default_branch: "main"
	}]
}
