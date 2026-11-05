package gcp

// The project every record names: the project ID the collector passed to
// gcloud --project, never the project number or display name. 6-30 lowercase
// letters, digits and hyphens, starting with a letter and not ending with a
// hyphen; legacy IDs may carry a domain prefix (example.com:my-project).
_gcpProjectID: =~#"^([a-z0-9][a-z0-9.-]*[a-z0-9]:)?[a-z][a-z0-9-]{4,28}[a-z0-9]$"#
