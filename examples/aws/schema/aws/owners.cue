package aws

// Ownership. Every type declares `_pudl: owner`: a resource belongs to its
// account's team in _awsAccountOwners, else to nobody. Unowned resources are
// the resource_unowned relation (`pudl query resource_unowned`); `pudl check
// --owner TEAM` shows one team's findings.
//
// AWS tags are a list of {Key, Value} pairs (Tags, or TagList on RDS), and an
// owner path names one value by field path: it cannot select the list element
// whose Key is "Owner" or "Team". So the pack maps accounts, not tags. Name
// the team of each account you collect, here or in a file of your own in this
// package:
//
//	_awsAccountOwners: {"123456789012": "platform", "210987654321": "data"}
_awsAccountOwners: {[string]: string}

// _#AWSOwner is the owner declaration every type shares.
_#AWSOwner: {
	map: {path: "AccountId", owners: _awsAccountOwners}
}
