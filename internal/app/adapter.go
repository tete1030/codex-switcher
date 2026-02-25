package app

type Adapter interface {
	Tool() ToolName
	Inspect(paths ToolPaths) (InspectToolResult, error)
	ReadActiveCredential(paths ToolPaths) (Credential, bool, error)
	WriteActiveCredential(paths ToolPaths, cred Credential) error
	ClearActiveCredential(paths ToolPaths) error
}

func adapterFor(tool ToolName) Adapter {
	switch tool {
	case ToolCodex:
		return &codexAdapter{}
	case ToolOpenCode:
		return &openCodeAdapter{}
	case ToolOpenClaw:
		return &openClawAdapter{}
	default:
		return nil
	}
}
