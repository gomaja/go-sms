module github.com/gomaja/go-sms

require github.com/stretchr/testify v1.12.1

require go.yaml.in/yaml/v3 v3.0.5 // indirect

go 1.23

retract (
	v1.0.2 // Published only to retract v1.0.1; depend on the main branch.
	v1.0.1 // Predates the fixes and the current API; depend on the main branch.
)
