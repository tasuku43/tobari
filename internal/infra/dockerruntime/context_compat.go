package dockerruntime

// activeContextDocument keeps the unpublished predecessor migration adapter
// buildable until the following migration commit switches to the dedicated
// legacy decoder. It is not a public or persisted Domain Model V1 type.
type activeContextDocument = legacyActiveContextDocument
