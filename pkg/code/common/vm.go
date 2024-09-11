package common

var (
	// The well-known Code VM instance used by the Code app
	//
	// todo: real public key once program is deployed and VM instance is initialized
	CodeVmAccount, _ = NewAccountFromPublicKeyString("BkwoMG33cgSDrc3fEjfhZufqzYC3icXTTMajuueXyYGG")

	// The well-known Code VM instance omnibus used by the Code app
	//
	// todo: real public key once program is deployed and VM omnibus instance is initialized
	CodeVmOmnibusAccount, _ = NewAccountFromPublicKeyString("SqKpQBYg8H69c8dusmSoLsya281cqhfzDVR2EJFHy1P")
)
