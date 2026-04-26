package application

// ImportOption configures a single ImportPasswordCredential call. Unlike
// AuthServiceOption (which wires service-wide dependencies once at
// construction), ImportOption carries per-credential state — the legacy
// salt suffix in particular is a per-row value that varies between
// migrated records.
type ImportOption func(*importConfig)

// importConfig holds the parsed options for a single
// ImportPasswordCredential call. Kept unexported so callers can only
// build it through the documented With* constructors.
type importConfig struct {
	saltSuffix string
}

// ImportWithSalt attaches a plaintext salt suffix to the imported
// credential. The salt is appended to the user-supplied plaintext before
// bcrypt comparison on every subsequent VerifyPassword call, allowing
// import of legacy hashes whose plaintext was suffixed before hashing.
//
// New credentials produced by RegisterPassword never carry a salt —
// pericarp relies on bcrypt's own per-hash salt — and rotating a
// password via UpdatePassword permanently clears any imported salt.
func ImportWithSalt(salt string) ImportOption {
	return func(c *importConfig) { c.saltSuffix = salt }
}
