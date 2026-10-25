# Requested extension capabilities

Updated: 2026-10-25.
This backlog records the configuration-page discussion for continued development on Linux.
The current build covers repository integration, versioning, CLI compatibility, release packaging, release automation, upgrade guidance, and guided CLI installation and update actions.
The configuration page remains unimplemented and outside that build.
This document records desired scope; it does not authorize every suggestion or start a separate implementation task.

## Explicit user requests

The user wants to configure TADX through the extension, including these actions:

| Area | Requested capability |
| --- | --- |
| Environments | Add a saved environment. |
| Workspaces | Add a workspace. |
| Defaults | Change saved environment and workspace defaults. |
| Mutations | View and change saved mutation consent for the selected Tableau site. |

Adding a workspace needs a clear choice between creating a managed workspace and registering an existing workspace directory.
Changing a saved default must remain distinct from selecting an environment or workspace to browse.
Expanding a site or changing the bulk-action source must not change CLI defaults.

## Suggested configuration-page scope

The proposed page has **Environments**, **Workspaces**, and **TADX CLI** sections.
These additions were recommended during discussion and need scope confirmation before implementation.

| Section | Suggested additions |
| --- | --- |
| Environments | Edit and remove saved environment profiles; display the server URL, site, and default status. |
| Authentication | Configure or replace PAT credentials, display authentication status, and sign out through CLI-owned operations. |
| Connection checks | Test the selected connection and explain connectivity, authentication, and permission failures. |
| Mutation settings | Display saved site consent separately from restrictions imposed by administrator policy. |
| Workspaces | Offer **Use Current Folder**, folder selection, and reveal-in-file-manager actions. |
| Workspace health | Identify missing folders, invalid workspace metadata, and registration problems. |
| TADX CLI | Display the active executable, version, compatibility result, and local or remote workspace host. |
| CLI setup | Expose the current build's executable selection, guided installation, and explicitly requested update actions within the future configuration page. |
| Diagnostics | Open diagnostic output and copy a redacted troubleshooting summary. |

The current release work includes compatibility, upgrade infrastructure, and guided CLI installation and update buttons.
Embedding its status and setup actions in a configuration page is a separate UI feature.
The user explicitly included those setup buttons in the current build on 2026-10-25.

Use lists with **Default** badges and contextual actions for environments and workspaces.
Selecting an environment can open its connection, authentication, and mutation settings.
Keep mutation changes explicit and show the canonical server, exact site, and persisted setting before confirmation.

## Cleanup decisions

The user explicitly chose to keep **Delete Local** on individual artifacts.
Do not add bulk deletion to this scope.

**Remove Workspace**, workspace unregistration, whole-workspace deletion, and **Cleanup Workspace** remain undecided.
Do not present them as approved configuration-page requirements.
If revisited, distinguish removing a registration from deleting workspace files before choosing labels or behavior.
Recoverable trash and automatic cleanup are not approved requirements.

## Implementation boundaries

Use the public CLI for configuration, workspace registration, authentication, and mutation consent.
Do not edit private configuration files, open SQLite, or add Tableau API access in the extension.
Keep native credential persistence in the CLI and store only its opaque credential references in configuration.
Obtain explicit user approval before saving a PAT in the native credential store.
Never place secrets in command arguments, logs, webview state, diagnostics, fixtures, or the handoff.
Confirm a secure public input contract before designing credential entry; extend the CLI if that contract is missing.

Mutation consent applies to the canonical server and exact site, with an explicit persisted setting.
A connection test, environment selection, preview, or general setup request does not authorize enabling mutations.
The extension must preserve administrator policy restrictions and explain them without offering a bypass.

Prefer one shared user CLI installation for the extension, terminal, and coding agents.
Preserve a custom executable selection and keep an older compatible CLI usable.
Perform setup on the workspace host for SSH, WSL, and container sessions.
Coordinate executable updates with active transfers and shared state formats.
Binary rollback alone does not establish compatibility with changed cache or workspace data.

Before implementation, agree on the first configuration-page slice and define end-user tests for that slice.
Use the [extension architecture](../../architecture/editor-extension.md) for component ownership and the [agent entry prompt](START-HERE.md) for handoff constraints.
