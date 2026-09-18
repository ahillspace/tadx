# CLI shorthand

TADX keeps every canonical command, action, and flag available.
Shorthand is an optional spelling for frequently used names.
Canonical names remain the stable form for scripts, generated artifacts, structured output, and registry IDs.

## Command and action aliases

The following aliases are supported by the CLI command tree.

| Canonical | Alias | Meaning |
| --- | --- | --- |
| `admin` | `adm` | Administration command group. |
| `group-member` | `gm` | Direct group membership resource. |
| `label-value` | `lv` | Shared label value definitions. |
| `label-category` | `lc` | Shared label categories. |
| `agent` | `agt` | Agent command group. |
| `artifact` | `art` | Artifact command group. |
| `auth` | `ath` | Authentication command group. |
| `capability` | `cap` | Capability command group. |
| `cache` | `cch` | Cache command group. |
| `check` | `chk` | Check action. |
| `clean` | `cln` | Clean action. |
| `clone` | `cl` | Clone action. |
| `completion` | `cmp` | Completion command group. |
| `content` | `con` | Content command group. |
| `create` | `new` | Create action. |
| `datasource` | `ds` | Datasource resource. |
| `default` | `dft` | Default action. |
| `definition` | `def` | Pulse definition resource. |
| `delete` | `del` | Delete action. |
| `doctor` | `doc` | Doctor command group. |
| `flow` | `flw` | Flow resource. |
| `follow` | `fol` | Follow action. |
| `followers` | `fls` | Followers action. |
| `fork` | `frk` | Fork action. |
| `group` | `grp` | Group resource. |
| `inspect` | `ins` | Inspect action. |
| `install` | `ist` | Install action. |
| `last` | `lst` | Show the previous saved result. |
| `lineage` | `lin` | Lineage command group. |
| `list` | `ls` | List action. |
| `login` | `log` | Login action. |
| `logout` | `out` | Logout action. |
| `member` | `mem` | Member command group. |
| `metric` | `met` | Metric resource. |
| `move` | `mv` | Move action. |
| `mutation` | `mut` | Mutation command group. |
| `permission` | `prm` | Permission resource. |
| `project` | `prj` | Project resource. |
| `publish` | `pub` | Publish action. |
| `pull` | `pl` | Pull action. |
| `pulse` | `pls` | Pulse command group. |
| `refresh` | `ref` | Refresh action. |
| `register` | `reg` | Register action. |
| `remove` | `rm` | Remove action. |
| `schema` | `sch` | Schema action. |
| `search` | `sea` | Search action. |
| `set-default` | `sdf` | Set-default action. |
| `status` | `st` | Status action. |
| `uninstall` | `uni` | Uninstall action. |
| `unfollow` | `unf` | Unfollow action. |
| `unregister` | `unr` | Unregister action. |
| `update` | `upd` | Update action. |
| `user` | `usr` | User resource. |
| `version` | `ver` | Version command group. |
| `workbook` | `wb` | Workbook resource. |
| `workspace` | `ws` | Workspace command group. |

Aliases compose at each command level.
For example, `tadx con ds ls` is equivalent to `tadx content datasource list`.
Use the canonical spelling when an alias is not listed or when command text is persisted for long-term use.
Names already three characters or shorter stay unchanged.
Built-in `help` retains its conventional spelling and `-h` flag; shell names and other argument values are not abbreviated.

## Flag aliases

Single-letter flags use one dash.
Word-like aliases use two dashes, even when the alias is short.

| Canonical flag | Alias | Meaning |
| --- | --- | --- |
| `--aggregation` | `--agg` | Aggregation. |
| `--all` | `-a` | All matching records within the documented bound. |
| `--api-version` | `--api` | API version. |
| `--append` | `--apd` | Append mode. |
| `--artifact` | `--art` | Workspace-relative artifact. |
| `--artifact-name` | `--arn` | Artifact name. |
| `--auth-setting` | `--aus` | Authentication setting. |
| `--capability` | `--cap` | Capability name. |
| `--cache` | `--cch` | Read the local cache. |
| `--cache-max-concurrency` | `--cmc` | Cache concurrency ceiling. |
| `--check` | `--chk` | Check selector. |
| `--class` | `--cls` | Class selector. |
| `--clear-api-version` | `--cav` | Clear API version. |
| `--clear-cache-max-concurrency` | `--ccm` | Clear cache concurrency. |
| `--clear-default-workspace` | `--cdw` | Clear default workspace. |
| `--clear-pat-name-env` | `--cpn` | Clear PAT name variable. |
| `--clear-pat-secret-env` | `--cps` | Clear PAT secret variable. |
| `--clear-site` | `--cst` | Clear site. |
| `--config` | `--cfg` | Configuration path. |
| `--content-permissions` | `--cpm` | Content permission mode. |
| `--create` | `--new` | Create mode. |
| `--currency` | `--cur` | Currency code. |
| `--cursor` | `--csr` | Provider cursor. |
| `--datasource-id` | `--did` | Datasource LUID. |
| `--datasource-map` | `--dsm` | Datasource identity mapping. |
| `--date-field` | `--dtf` | Date field. |
| `--days` | `--day` | Number of days. |
| `--default-for` | `--dft` | Default permission target. |
| `--default-workspace` | `--dws` | Default workspace. |
| `--definition-id` | `--dfi` | Definition LUID. |
| `--depth` | `--dep` | Lineage depth. |
| `--description` | `--dsc` | Description. |
| `--destination` | `--dst` | Destination. |
| `--destination-project` | `--dpj` | Destination project path. |
| `--destination-project-id` | `--dpi` | Destination project LUID. |
| `--dimension` | `--dim` | Dimension field. |
| `--direction` | `--dir` | Lineage direction. |
| `--domain` | `--dom` | Group domain. |
| `--email` | `--eml` | Email address. |
| `--enabled` | `--ena` | Enabled state. |
| `--environment` | `-e`, `--env` | Configured environment. |
| `--exclude-filter` | `--exf` | Excluded filter. |
| `--external-user-enabled` | `--eue` | External-user setting. |
| `--field-id` | `--fid` | Field ID. |
| `--file` | `--fil` | Native file. |
| `--filter` | `--flt` | Filter. |
| `--force` | `--frc` | Explicit safety confirmation. |
| `--full` | `-f`, `--ful` | Expanded bounded output. |
| `--full-name` | `--fnm` | Full name. |
| `--group-id` | `--gid` | Group LUID. |
| `--id` | `-i` | Exact primary resource LUID. |
| `--identity-pool` | `--idp` | Identity pool. |
| `--idp-configuration-id` | `--ici` | Identity-provider LUID. |
| `--include-extract` | `--iex` | Include extract. |
| `--include-pds` | `--ipd` | Include published datasources. |
| `--kind` | `--knd` | Resource kind. |
| `--language` | `--lng` | Language. |
| `--limit` | `-l`, `--lim` | Result bound. |
| `--locale` | `--loc` | Locale. |
| `--measure-field` | `--msf` | Measure field. |
| `--member-id` | `--mid` | Member LUID. |
| `--members` | `--mem` | Include members. |
| `--minimum-granularity` | `--mng` | Minimum granularity. |
| `--minimum-site-role` | `--msr` | Minimum site role. |
| `--mode` | `--mod` | Permission mode. |
| `--mutation` | `--mut` | Mutation setting. |
| `--name` | `-n`, `--nm` | Exact name. |
| `--new-name` | `--nnm` | Replacement name. |
| `--number-format` | `--nfm` | Number format. |
| `--overwrite` | `--ovr` | Overwrite existing content. |
| `--owner` | `--own` | Owner. |
| `--owner-id` | `--oid` | Owner LUID. |
| `--parent` | `--par` | Parent path. |
| `--parent-id` | `--pai` | Parent LUID. |
| `--path` | `--pth` | Path. |
| `--pat-name-env` | `--pne` | PAT name variable. |
| `--pat-secret-env` | `--pse` | PAT secret variable. |
| `--period` | `--per` | Period. |
| `--preview` | `-p`, `--pv` | Read-only mutation plan. |
| `--principal-id` | `--pri` | Principal LUID. |
| `--principal-type` | `--prt` | Principal type. |
| `--product` | `--prd` | Product. |
| `--project` | `--prj` | Project path. |
| `--project-id` | `--pid` | Project LUID. |
| `--project-name` | `--pnm` | Project name filter. |
| `--query` | `-q`, `--qry` | Query. |
| `--replace` | `--rpl` | Replace mode. |
| `--resource` | `--res` | Resource. |
| `--role` | `--rol` | Role. |
| `--running-total` | `--rnt` | Running-total setting. |
| `--scope` | `--scp` | Cache scope. |
| `--sentiment` | `--snt` | Sentiment. |
| `--set-members` | `--stm` | Replace direct membership. |
| `--site` | `--sit` | Site. |
| `--site-role` | `--srl` | Site role. |
| `--source` | `--src` | Source. |
| `--subscription-id` | `--sid` | Subscription LUID. |
| `--table` | `--tbl` | Table. |
| `--target` | `--tgt` | Target. |
| `--temporality` | `--tmp` | Temporality. |
| `--top-level` | `--top` | Top-level project. |
| `--type` | `--typ` | Resource type. |
| `--updated-after` | `--uaf` | Lower update bound. |
| `--updated-before` | `--ubf` | Upper update bound. |
| `--user-id` | `--uid` | User LUID. |
| `--workspace` | `-w`, `--ws` | Registered workspace. |

Only flags accepted by the selected leaf command apply.
`-f` always means `--full`.
It does not mean `--force`.
Use the canonical `--force` flag where a command requires an explicit safety confirmation.
Shorthand does not bypass mutation policy.
`--preview` remains read-only and does not authorize execution.
Name-based deletion requires both an exact name and an exact project path; use `--id` instead when the LUID is known.
The list filter `--project-name` (`--pnm`) matches an exact project leaf name, not a project LUID or hierarchy path.

## Shell completion

Completion accepts command aliases when navigating the command tree.
Suggestions use canonical command and flag names plus supported single-letter flags.
Long flag aliases such as `--nm` and `--pv` remain accepted input but are not separate completion candidates.

## Examples

```text
tadx con ds ls --env dev --pnm "Analytics" -l 50
tadx con ds del --env dev --nm "Revenue" --prj "Analytics" -p -f
tadx con wb ls --env dev -a
```

These examples use a generic environment alias and an exact project name.
Run `tadx <command> --help` when a shorthand appears unavailable or could be ambiguous.
