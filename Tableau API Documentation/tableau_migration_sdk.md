# Tableau Migration SDK - Documentation, Articles, and Samples

Drafting the full documentation, tutorials, and runnable example code from the official Migration SDK sites.

## Contents

- [Migration SDK Documentation](#section-1)
- [Installation](#section-2)
- [Planning the Migration](#section-3)
- [Architecture](#section-4)
- [Concepts and Terms](#section-5)
- [Supported Content Types](#section-6)
- [How to Migrate](#section-7)
- [Designing Your Migration Application](#section-8)
- [Running the Migration](#section-9)
- [Post-Migration Tasks](#section-10)
- [Best Practices](#section-11)
- [Troubleshooting](#section-12)
- [What's New in the Migration SDK](#section-13)

---

# Tableau Migration SDK Overview

- Migration SDK Overview
    - What should the Migration SDK be used for?
    - Who can use the Migration SDK?
    - When to use the Migration SDK
    - When not to use the Migration SDK
- Migration blockers
- Platform differences
- Technical limitations
- More information

## Migration SDK Overview

Customers worldwide are migrating to Tableau Cloud to take advantage of the innovation, reduced cost, security, and scalability of Tableau’s managed data service. The [Tableau Migration Software Development Kit (SDK)](https://github.com/tableau/tableau-migration-sdk) helps you migrate to Tableau Cloud in a seamless and predictable way. You can use the Migration SDK to build your own migration application to do the following:

- Migrate users and groups
- Migrate content like data sources and workbooks
- Migrate and, in some cases, modify your robust governance structure for your content in Tableau Cloud

With any migration project, you can also:

- Migrate content items like Workbooks, Data Sources and Projects from Tableau Server to Tableau Cloud
- Capture logs of essential actions taken
- Transform essential aspects of users, content, and governance in-flight

The Migration SDK gives you the tools to go from “lift and shift” methodologies to optimization in a Cloud-native environment. We’ve enriched the documentation and instructions to highlight opportunities to optimize your Tableau Cloud deployment for simplicity of administration, automation, and enriched data cultures following a migration.

### What should the Migration SDK be used for?

- The Migration SDK is used for the technical movement of users and content during a migration from Tableau Server to Tableau Cloud.
- The migration should be a one-time event rather than a recurring content promotion (such as a migration from development to production).
- The Migration SDK is a framework for your custom-coded projects. The SDK is not a standalone application that performs Tableau Server to Tableau Cloud migrations with no coding required.

### Who can use the Migration SDK?

The Migration SDK was designed for junior level developers and above who are familiar with Tableau and have experience in either Python or .NET. The Migration SDK gives you the tools to develop your own migration application. More details on the supported platforms and hardware requirements can be found in Installation.

### When to use the Migration SDK

The Migration SDK is used for the technical movement of users and content during a migration from Tableau Server to Tableau Cloud. The following image is helpful if you’re thinking about migrating to Tableau Cloud and where the Migration SDK fits into that process. This image shows an overview of the various teams and decisions that are needed for a successful migration.

### When not to use the Migration SDK

1. We don’t recommend that all Tableau Server customers migrate to Tableau Cloud. This is because of compliance or use case compatibility. Be aware of the [differences between Tableau Server and Tableau Cloud](https://help.tableau.com/current/server/en-us/migrate_server_to_cloud_overview.htm) and confirm your plans with your accounts teams before migrating to Tableau Cloud.
2. If you have fewer than 100 users, we recommend using the [Tableau Cloud Manual Migration Guide](https://help.tableau.com/v0.0/guides/migration/en-us/emg_intro.htm) instead. Customers fitting this profile usually migrate faster and don’t require development investment using a manual method. On average, this process usually takes customers of this size under two weeks to migrate to Cloud.
3. If you have more than 100 users and do not have internal resources familiar with Python or C#, we recommend leaning on [Tableau Professional Services](https://www.tableau.com/resources/teams-organizations/professional-services) or our [Experienced Migration Partners](https://www.tableau.com/why-tableau/tableau-migration).

## Migration blockers

While most customers will benefit from migrating from Tableau Server to Tableau Cloud, there are a few unique scenarios that might require you to continue to use Tableau Server. Check with your Tableau account team or a Partner for advice, or consider [potential migration obstacles](https://help.tableau.com/current/server/en-us/migrate_server_to_cloud_overview.htm).

## Platform differences

While Tableau Server and Tableau Cloud are remarkably similar, there are some differences between the two products that impact how you migrate. We call out these differences within specific migration steps in the Migration SDK documentation. However, the following links offer some high-level items to keep in mind when you’re planning your migration.

- [Summary of platform differences](https://help.tableau.com/current/server/en-us/migrate_server_to_cloud_overview.htm)
- [Version differences](https://public.tableau.com/app/profile/tableau.core.product.marketing/viz/ReleaseNavigator-V21/FeaturesInVersionDash)

## Technical limitations

The Migration SDK is a framework for your custom-coded projects. The SDK *is not* a standalone application that performs Tableau Server to Cloud migrations with no coding required.

The Migration SDK relies exclusively on REST API functionality to migrate content. If a content item doesn’t yet have the necessary REST API functionality, those items could need to be migrated manually by end users. We have detailed these items in Data not support by the Migration SDK.

## More information

For more information about manual processes, see the [Tableau Cloud Manual Migration Guide](https://help.tableau.com/current/guides/migration/en-us/emg_intro.htm).

---

# Install the Migration SDK

## Software requirements

The Migration SDK is available for Python and .NET, each supporting Linux, Mac, and Windows. Depending on the language, installations might differ.

- Python: 3.10 or later
- [.NET Runtime](https://dotnet.microsoft.com/en-us/download): 8.0 or later

## Hardware requirements

The Migration SDK downloads copies of content items onto the machine that the Migration SDK is installed on. Make sure there’s enough disk space on the machine to sequentially download content during the migration process.

## Installation instructions

### Python

To use the Migration SDK for Python, download the [Python package](https://pypi.org/project/tableau-migration/). For information about installing Python packages, see [Installing Packages](https://packaging.python.org/en/latest/tutorials/installing-packages/) in the Python documentation.

### .NET

To use the Migration SDK for .NET, download the [.NET NuGet package](https://www.nuget.org/packages/Tableau.Migration). You can import the `.nupkg` file locally into a .NET project and referenced in your code.

---

# Plan Your Migration

- Migration Strategy
    - Establish your migration goals
    - Choose a migration strategy
    - Ownership Strategy
    - Migration rollout strategy
- Roles and responsibilities
- Data not supported by the Migration SDK

## Migration Strategy

We strongly recommend that you commit to a migration plan with stated goals.

### Establish your migration goals

We recommend that you establish concrete goals for your migration. If your goals aren’t established before the migration, you can’t know if the migration is a success. The following examples are provided to simplify the process using [SMART goals](https://en.wikipedia.org/wiki/SMART_criteria).

- Deprecate Tableau Server within N weeks.
- Reduce Tableau’s total cost of ownership by X by migrating to Tableau Cloud within N weeks
- Migrate N% of Tableau Server content and verify it’s functional on Tableau Cloud within Y weeks.

### Choose a migration strategy

Read through the Ownership Strategy and Migration Rollout Strategy in this topic to determine which of the following strategies you will use to migrate to Tableau Cloud. Before your migration, you must have stakeholder commitment on the strategy. Without this commitment, your migration will suffer the problems of scope creep and extended timelines.

Select your preferred combination of Ownership and Rollout Strategies:

|  | Ownership Strategy |  |  |  |
|---|---|---|---|---|
| Centralized | Organizational Segmentation | Site-by-Site |  |  |
| Migration Rollout Strategy | All at Once |  |  |  |
| Phased |  |  |  |  |

### Ownership Strategy

There are three strategies for determining who fills each of the required roles during a Tableau Server to Tableau Cloud migration. Before migrating to Tableau Cloud, make sure you’re aligned on your desired ownership structure, especially if multiple teams are involved.

1. **Centralized ownership**: A single person or team is responsible for the migration
2. **Organizational segmentation**: Distributed responsibilities by business unit or team
3. **Site-by-site**: Distributed responsibilities by existing Tableau Server Site structure

| **Strategy** | **Pros** | **Cons** |
|---|---|---|
| Centralized ownership | Simplest, least coordination, managed centrally | Distribution of effort can be challenging at scale |
| Organizational segmentation | Roles and responsibilities are distributed across business units. Greatest opportunity for content optimization | Requires the most coordination, subject to internal org restructures |
| Site-by-site | Role clarity driven by content structure, can use site admins to drive migration and testing efforts | Cross-organizational collaboration must be agreed to early. Unclear or shifting priorities can cause disruption. |

#### Centralized ownership

This strategy is normally employed by customers with a single site and fewer than 500 Users. Not every organization that fits those qualifications use Centralized structures, but it becomes less common as Tableau Server deployments grow due to matrixed ownership of content.

We recommend this strategy as the simplest strategy, if your migration lead is able to individually connect with essential leaders across the entirety of the Tableau Server deployment to conduct a migration to Tableau Cloud.

#### Organizational segmentation

This strategy is normally employed by customers with more than one site and more than 500 Users. It relies on existing organizational structures to distribute roles and responsibilities. For example, an organization with three different business units participating in a migration might choose to each have duplicate roles in a migration if each business conducts their migration at different times.

We recommend this strategy if ownership of content is highly matrixed and different business units will be migrating in slightly different ways.

#### Site-by-site

This strategy is generally employed by customers with more than one site and more than 500 Users. Using this model, you distribute responsibilities to personnel in every site so that they can migrate to Tableau Cloud when it makes sense for that Tableau Server site to migrate. This model allows you to use the existing content governance structure in Tableau Server to distribute roles and responsibilities.

We recommend this strategy if you have highly matrixed ownership of content and different sites will be migrating in slightly different ways.

### Migration rollout strategy

There are generally three rollout strategies for conducting a migration.

1. **All at once**: The entirety of Tableau Server content is migrated in a single sprint.
2. **Phased**: Subsections of content are migrated across multiple sprints or teams based on business unit, or Server Site structure.

#### All at once

This strategy is often chosen for customers with fewer than 500 Users. It’s clearly the simplest model, but is harder to manage if there are multiple teams across varied business units who are each responsible for subsections of the migration. These groups can still use this strategy as long as they commit to roles, timelines, and strong communication early in the migration.

#### Phased

This strategy is often chosen by customers with more than 500 Users who for one reason or another can’t migrate all of their content in a single sprint. Factors that could impact this include varied stakeholders with different goals, timelines, or data strategy.

## Roles and responsibilities

The following are the common roles and responsibilities that should be agreed upon before your migration. Based on the size of the migration and the strategy employed, you might find that a role isn’t required, that a role requires multiple people or that a single person spans multiple roles. Before you begin migrating, make sure you have individuals assigned to each of the below roles.

| Role | Definition | Owner |
|---|---|---|
| Executive Sponsor | Responsible for the outcome of the migration and ensuring each role is properly staffed. |  |
| Project Manager | Responsible for timelines, coordination, and delegation of ownership |  |
| Technical Lead | Technical lead for using the Migration SDK |  |
| Readiness Assessment | Responsible for ensuring all content is fit for use in Tableau Cloud |  |
| Tableau Cloud Site Admin | Responsible for administration of the Tableau Cloud deployment during and post-migration. There could be more than one site admin. However, we recommend having one person ultimately responsible during the duration of a migration. |  |
| Testing Lead | Responsible for ensuring content is tested on Tableau Cloud before the go-live date. |  |
| Tableau Cloud Project Leaders | Primarily an informed role, although often provide feedback on migration progress on subsections of content. This role most commonly has more than one person listed as an owner. But it's helpful to identify these people ahead of time so that they understand their accountability to the content in their Project(s). |  |

## Data not supported by the Migration SDK

You must manually migrate these items:

- Ask Data Lenses (Retired in Tableau Cloud February 2024 / Server 2024.2)
- Collections
- Comments
- Data policies (Data Management)
- Data quality warnings
- Data roles (Data Management)
- Data-driven alerts
- Embedded solutions
- Extensions
- Metrics (Retired in Tableau Cloud February 2024 / Server 2024.2)
- Personal spaces (all content needs to be published publicly to be migrated)
- Prep flows (Data Management)
- Revisions
- Server or site customizations
- URL actions (Workbook or Data Source transformation required)
- Usage and engagement data
- User filters (Workbook or Data Source transformation required)
- User settings
- Virtual connections (Data Management)

The Tableau Catalog isn’t migrated by the Migration SDK. However, after content is in Tableau Cloud, it’s indexed by Tableau Catalog and is usually available within 48 hours of an asset migration.

For data supported by the Migration SDK, see Content types.

---

# Architecture

The Migration SDK is designed as a local application that primarily downloads content from Tableau Server to a local machine and then publishes it to Tableau Server. The Migration SDK exclusively uses the [Tableau REST API](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api.htm) to migrate content.

```
graph LR
A <--> B
C <--> B
B <--> D
subgraph Tableau Server
A[APIs]
end

subgraph User Application
B[Migration SDK]
end

subgraph Tableau Cloud
C[APIs]
end

D[(Temporary storage)]
```

## API Throttling

Tableau Cloud has published [Site Capacity limits](https://help.tableau.com/current/online/en-us/to_site_capacity.htm) to ensure all customers have a consistent performance experience. This includes limits on how many API calls a user can make on a specific Tableau Cloud Site within an hour.

The Migration SDK is limited by Tableau’s throttling limits for REST API operations. Tableau Cloud limits REST API usage to 40,000 calls per user per site per hour. Additionally, select REST API calls have lower limits detailed in [Tableau Cloud Site Capacity](https://help.tableau.com/current/online/en-us/to_site_capacity.htm).

If a throttling limit is hit, API operations will be paused until the next hour. Because of this, you might notice delays in the migration progress. This is normal and expected.

## Security

The Migration SDK relies on HTTPS to secure data in transit. Tableau Cloud API connections require HTTPS, and we recommend configuring HTTPS to connect to Tableau Server when available. The Migration SDK encrypts downloaded files by default to secure data at rest.

The Migration SDK requires a personal access tokens (PAT) to authenticate the user running the migration with Tableau APIs. These tokens must be created on both Tableau Server and Tableau Cloud. For more information, see [Personal Access Tokens](https://help.tableau.com/current/online/en-us/security_personal_access_tokens.htm) in the Tableau Help Documentation.

The Migration SDK does not persist PATs. We recommend that you securely store your tokens.

---

# Concepts

- Content Types
- Migration Plan
- Migration Plan Builder
- Migration Result
- Configuration

This topic details the terms and concepts you must understand before you plan your migration from Tableau Server to Tableau Cloud.

## Content Types

Content types are the various types of content that reside on Tableau Server or Tableau Cloud. For details about which content types the Migration SDK supports, see Supported Content Types.

For unsupported content types, see Data not supported by the Migration SDK.

## Migration Plan

A migration plan describes how a migration should be done and what customizations must be done inflight. See [IMigrationPlan](https://tableau.github.io/migration-sdk/api-csharp/Tableau.Migration.IMigrationPlan.html) in the API Reference documentation for a list of properties and their descriptions.

## Migration Plan Builder

This is the best way to build a migration plan. Calling the `Build()` method on the `IMigrationPlanBuilder` gives you a MigrationPlan. For more information, see [IMigrationPlanBuilder](https://tableau.github.io/migration-sdk/api-csharp/Tableau.Migration.IMigrationPlanBuilder.html) in the API Reference documentation for details.

## Migration Result

This is the result generated after the migration has finished. It has two properties:

- Migration Status: This is simply the status of the migration. See [MigrationCompletionStatus](https://tableau.github.io/migration-sdk/api-csharp/Tableau.Migration.MigrationCompletionStatus.html) enumeration in the API Reference documentation for various status items supported.
- Migration Manifest: The migration manifest describes the various Tableau data items found to migrate and their migration results. See [IMigrationManifest](https://tableau.github.io/migration-sdk/api-csharp/Tableau.Migration.IMigrationManifest.html) in the API Reference documentation for details.

## Configuration

Configuration can be done using the `MigrationSdkOptions`. For information about configuration items and their defaults, see [MigrationSdkOptions](https://tableau.github.io/migration-sdk/api-csharp/Tableau.Migration.Config.MigrationSdkOptions.html) in the API Reference documentation.

---

# Supported Content Types

The Migration SDK is equipped to migrate the following content types. If no filters are provided, items for all content types are migrated with some exceptions. Note the minimum permissions required to migrate each content type.

| Content Type | What is migrated | What isn’t migrated | Minimum permissions |
|---|---|---|---|
| [Custom Views](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_workbooks_and_views.htm#list_custom_views) | OwnershipList of users for whom the custom view is default. |  | **Source**: Site Administrator **Destination**: Site Administrator |
| [Data Sources](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_data_sources.htm) | OwnershipPermissions TagsLabelsEmbedded credentials, except for managed OAuth embedded credentials | Revision HistoryManaged OAuth embedded credentials. Data sources will appear as failed in the manifest if they contain managed OAuth embedded credentials. For more information, see [OAuth Connections](https://help.tableau.com/current/server/en-us/protected_auth.htm). | **Source**: Read (view), Connect, and ExportXml permissions on all data sources **Destination**: Site Administrator |
| [Extract Refresh Schedules](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_extract_and_encryption.htm) | Extract Refresh schedules with frequencies and intervals are migrated to cloud compatible schedules. Make sure you set the correct time zone. In the case where the time zones don't match, you must add a transformer to update the day/time of the scheduled task. Schedules with an hourly frequency and less than one hour intervals are migrated to have a one hour frequency. See [Schedule Refreshes on Tableau Cloud](https://help.tableau.com/current/online/en-us/schedule_add.htm#create-a-refresh-schedule) for details about supported frequencies and intervals. | Schedule Name Schedule Priority Schedule Execution Order (Parallel or Serial) Weekly frequency, with multiple days of the week selected For more information, see [Jobs, Tasks, and Schedule Methods](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_jobs_tasks_and_schedules.htm). | **Source**: Site Administrator **Destination**: Site Administrator |
| [Favorites](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_favorites.htm) | Data source favorites Project favorites Workbook favorites View favorites | Flow favorites | **Source**: Site Administrator **Destination**: Site Administrator |
| [Group Sets](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_users_and_groups.htm) | Group sets and group linkage to group sets. |  | **Source**: Site Administrator **Destination**: Site Administrator |
| [Groups](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_users_and_groups.htm) | Group membership for users. | Group “All Users” | **Source**: Site Administrator **Destination**: Site Administrator |
| [Projects](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_projects.htm) | Permissions Ownership |  | **Source**: Read (view) permissions on all projects **Destination**: Site Administrator |
| [Subscriptions](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_subscriptions.htm) | Subscription schedules with frequencies and intervals are migrated to cloud compatible schedules. Make sure you [set the correct time zone](https://help.tableau.com/current/api/migration_sdk/en-us/docs/how_to_migrate.html#set-time-zone). In the case where the time zones don't match, you must add a transformer to update the day/time of the subscription. Schedules with an hourly frequency and less than one hour intervals are migrated to have a one hour frequency. See [Set up subscriptions on Tableau Cloud](https://help.tableau.com/current/online/en-us/subscribe_user.htm#set-up-a-subscription-for-yourself-or-others) for details about supported frequencies and intervals. | Schedule Name Schedule Priority Schedule Execution Order (Parallel or Serial) Weekly frequency, with multiple days of the week selected For more information, see [Jobs, Tasks, and Schedule Methods](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_jobs_tasks_and_schedules.htm). | **Source**: Site Administrator **Destination**: Site Administrator |
| [Users](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_users_and_groups.htm) |  |  | **Source**: Site Administrator **Destination**: Site Administrator |
| [Workbooks](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_workbooks_and_views.htm) | OwnershipPermissions TagsEmbedded credentials, except for managed OAuth embedded credentials | DataAccelerationConfig Managed OAuth embedded credentials. Workbooks will appear as failed in the manifest if they contain managed OAuth embedded credentials. For more information, see [OAuth Connections](https://help.tableau.com/current/server/en-us/protected_auth.htm). Labels | **Source**: Read (view) and ExportXml permissions on all workbooks **Destination**: Site Administrator |
| [Prep Flows](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_flow.htm) | OwnershipPermissions TagsDescription Project location Embedded credentials, except for managed OAuth embedded credentials | Managed OAuth embedded credentials. Prep flows will appear as failed in the manifest if they contain managed OAuth embedded credentials. For more information, see [OAuth Connections](https://help.tableau.com/current/server/en-us/protected_auth.htm). Labels | **Source**: Read (view) and ExportXml permissions on all Prep flows **Destination**: Site Administrator |
| [Prep Flow Tasks](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_flow.htm) | Prep flow run task schedules (Server-to-Server, Server-to-Cloud, and Cloud-to-Cloud) with frequencies and intervals are migrated to cloud compatible schedules. Make sure you set the correct time zone. In the case where the time zones don't match, you must add a transformer to update the day/time of the scheduled task. Schedules with an hourly frequency and less than one hour intervals are migrated to have a one hour frequency. See [Schedule Refreshes on Tableau Cloud](https://help.tableau.com/current/online/en-us/schedule_add.htm) for details about supported frequencies and intervals. | Schedule Name Schedule Priority Schedule Execution Order (Parallel or Serial) Weekly frequency, with multiple days of the week selected For more information, see [Jobs, Tasks, and Schedule Methods](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_jobs_tasks_and_schedules.htm). | **Source**: Site Administrator **Destination**: Site Administrator |

---

# Pre-Migration Checklist

The [Tableau Migration Software Development Kit (SDK)](https://github.com/tableau/tableau-migration-sdk) requires that you have a licensed Tableau Cloud environment that is properly licensed and configured. To migrate smoothly, complete the tasks in this topic before you use the Migration SDK.

## Step 1: Upgrade your Tableau Server

Upgrade your existing installation of Tableau Server to the latest version. For more information, see the [upgrade instructions for Windows](https://help.tableau.com/current/server/en-us/upgrade.htm) or the [upgrade instructions for Linux](https://help.tableau.com/current/server-linux/en-us/upgrade.htm).

## Step 2: Create a Tableau Cloud site

The Migration SDK is compatible with both a licensed and trial Tableau Cloud Site. However, work with your account team to create a site that meets your user, content, and license type requirements.

## Step 3: Configure all required Tableau Cloud site settings

For more information, see [Customize the Site and Content Settings](https://help.tableau.com/current/online/en-us/to_customize_site_and_content.htm)

## Step 4: Configure Tableau Cloud user authentication

By default, Tableau Cloud sends a Site Invite Notification email to all users when they’re added to a site. If you want to disable that automated notification, disable the **Site Invite Notification** in **Settings** under the General tab.

For more information, see [Authentication](https://help.tableau.com/current/online/en-us/security_auth.htm).

## Step: 5: Set the correct time zone

The Migration SDK doesn’t consider the time zone setting. You must set the correct time zone on the destination site.

## Step 6: Configure Tableau Server to allow embedded credential migration

Use the following steps to allow embedded credential migration from Tableau Server to Tableau Cloud.

### In Tableau Cloud:

1. Sign in to Tableau Cloud as a site administrator.
2. Select **Settings > General**, and scroll down to **Manage Content Migration**.
3. Click **Create new key** to generate an encryption key pair. **Note**: The public key is only displayed once. If you lose the key before completing the configuration, you must generate a new key.
4. In the resulting window, click **Copy to clipboard** and then close the window.
5. Paste the public key to a file and store it in a safe location. The TSM administrator will use the public key to allow migration. You can view the public key expiration date on the **Settings** page.

### In the TSM CLI:

1. Open a Windows Command Prompt with an account that is a member of the Administrators group on a node in the cluster.
2. Use `tsm security authorize-credential-migration` to allow embedded credential migration to the Tableau Cloud site. For more information, see [tsm security](https://help.tableau.com/current/server/en-us/cli_security_tsm.htm). `tsm security authorize-credential-migration --source-site-url-namespace <Tableau Server site ID> --destination-site-url-namespace <Tableau Cloud site ID> --destination-server-url <Tableau Cloud site url> --authorized-migration-runner <username> --destination-public-encryption-key <public key> ` **Note**: When running TSM commands from a remote node, use `tsm login` to authenticate a session with the Tableau Server Administration Controller service before running `tsm security authorize-credential-migration`.

## Step 7: Configure database connectivity

Some Cloud-accessible databases require you to [authorize access to cloud data published to Tableau Cloud](https://help.tableau.com/current/pro/desktop/en-us/publish_tableau_online_ip_authorization.htm).

If Tableau Bridge is required:

1. [Install and configure Tableau Bridge](https://help.tableau.com/current/online/en-us/qs_refresh_local_data.htm)
2. [Configure Tableau Bridge client pools](https://help.tableau.com/current/online/en-us/to_enable_bridge_live_connections.htm) **Important**: Configure your own Bridge Pool. The Default Bridge Pool doesn’t support some aspects of the Migration SDK. For more information about Tableau Bridge, see [Plan Your Bridge Deployment](https://help.tableau.com/current/online/en-us/to_bridge_scale.htm)

## Step 8: Configure OAuth clients

For OAuth credentials to function, you must configure the destination site with any OAuth clients used on the source site. For more information, see [OAuth Connections](https://help.tableau.com/current/online/en-us/protected_auth.htm).

## Step 9: (Optional) Automate user provisioning and group synchronization

By default the Migration SDK migrates your groups exactly as they exist on Tableau Server. However it will not continue to sync your groups. If you want to create new groups during the migration, we recommend taking this step now, and opting to not migrate Groups using the Migration SDK.

For more information, see [Automate User Provisioning and Group Synchronization](https://help.tableau.com/current/online/en-us/scim_config_online.htm).

---

# Design Your Migration Application

The [Tableau Migration Software Development Kit (SDK)](https://github.com/tableau/tableau-migration-sdk) has a wide range of applications, but executes the migration in a multi-threaded manner within a single process. For best results, your migration application should be designed to perform long-running tasks.

The Migration SDK relies on the migration application to supply configuration and input to the migration engine, and to handle the results of the migration. Migration applications are responsible for persisting and loading configuration files, migration plan input, and migration results when desired.

---

# Run Your Migration

- Operation order
- Migration timing
- Understanding a migration run

Before you begin using the SDK, it is essential that you complete the Pre-Migration Checklist. If you don’t complte these items, the migration might not be successful.

## Operation order

To successfully migrate, you must understand and follow the order of operations that the Migration SDK uses. Many concepts build on each other and are prerequisites for later steps. For example, projects must be migrated to Tableau Cloud before workbooks in order for workbooks to be published into a project. See the following order of operations to see the order of operations of a migration.

When content is migrated from Tableau Server to Tableau Cloud, the Migration SDK will do a number of things to automate transformation in flight. One of these things is to map users across environments.

On Tableau Server, a username might not be an email. On Tableau Cloud, a username must be a valid email address. For example, John Smith may appear as jsmith on Tableau Server but will be johnsmith@tableau.com on Tableau Cloud. However, things like asset ownership and Tableau permissions assigned to John Smith must follow the user across both environments. We rely on user mapping to ensure that users own the same content on both environments and have access to the same content in both environments.

When a content item is migrated, it generally follows the same migration pattern. The following is a high-level overview of that pattern.

- List: Get a list of content items from the source.
- Map: Map source content items to destination content items.
- Filter: Filter any content items that should not be migrated. Outside of the default filters that the Migration SDK applies, everything is migrated if filters are not provided.
- Pull: Pull any additional information from the source that is required to publish the content item.
- Transform: Change certain properties within the content items to account for things such as server-cloud differences.
- Publish: Publish the content item prepared using the previous steps, to the destination.
- Post-Publish: Perform operations on the destination based so the content items correctly contain properties such as ownership, permissions etc.

There are known exceptions to this pattern such as batch operations, but those exceptions start from this framework as the baseline.

## Migration timing

Tableau Cloud undergoes periodic maintenance to sustain the infrastructure supporting Tableau Cloud services. Please review the [reserved system maintenance schedule](https://help.tableau.com/current/online/en-us/to_maintenance_schedule.htm#reserved-system-maintenance-schedule) to ensure you do not attempt a migration during reserved system maintenance windows. If Tableau Cloud is unavailable when you attempt a migration, the migration will fail.

Tableau Cloud undergoes major upgrades three times a year. We recommend that you plan your migration to complete before or after an upgrade window. See the [quarterly release schedule](https://help.tableau.com/current/online/en-us/to_maintenance_schedule.htm#quarterly-release-schedule) to understand more about Tableau Cloud upgrades and how to plan around them.

## Understanding a migration run

To help you understand what took place in a migration run, the Migration SDK outputs a manifest file as part of the migration result. This manifest catalogs the items that successfully migrated to Tableau Cloud and their current destination on the site.

Logging is also available for the Migration SDK. For more details, see the [Migration SDK API Reference documentation](https://tableau.github.io/migration-sdk/api-csharp/Tableau.Migration.Interop.Logging.html).

---

# Complete Post-Migration Tasks

After a successful run of the Migration SDK, there are other tasks you must perform before you onboard your users. This topic lists these tasks.

- Step 1: Validate migrated content
    - Did the right content migrate?
    - Is the content in the right place?
    - Does the content have the right permissions?
    - Is the data within the dashboard the same?
    - Are my dashboards performing well?
- Step 2: Recreate or manually migrate all items that aren’t programmatically migrated to Tableau Cloud.
- Step 3: Onboard end users

## Step 1: Validate migrated content

After your migration, there are five areas of validation to consider. You might not choose to invest in all five of these areas if your migration process can be manually validated quickly, or if you were covered in a proof-of-concept before migration. If you decide these areas are important for your organization, we have provided recommended processes to streamline those efforts.

### Did the right content migrate?

- **Content validation for an individual migration run**: With the Migration SDK’s Manifest, you receive a full readout of the items migrated to Tableau Cloud in the latest migration. Review the assets listed in this file to confirm if the intended items were migrated. [Learn more about the Manifest](https://tableau.github.io/migration-sdk/api-csharp/Tableau.Migration.Engine.Manifest.html).
- **Content validation across multiple runs**: If you would prefer to review all items on Tableau Cloud, you can use the Site Content data source in [Admin Insights](https://help.tableau.com/current/online/en-us/adminview_insights.htm). This data source allows you to build a custom workbook that shows all content on your Site. We’ve provided the following sample view. Note that Admin Insights data sources update daily or weekly depending on your Settings configurations. You’ll need to wait until the data updates to get the most up-to-date view on your Site Content. **Note**: Admin Insights data sources update daily or weekly depending on your Settings configurations. You must wait until the data updates to get the most up-to-date data on Viz Load Times.

### Is the content in the right place?

- **Validating the location of an individual content item**: The easiest way to verify the location of an individual content item is to review the item path in the UI. Using the Quick Search bar or manual navigation, go to a content item. After you have navigated to a content item, you will see the navigation path displayed above the name of the item like the following example. In this case, the “Executive Overview Workbook” item is in the “Sales Leadership” Project, which is in the “Sales-Production” Project.
- **Validating the location of multiple content items**: For a holistic understanding of where content is located across your Tableau Cloud Site, see Content validation across multiple runs.

### Does the content have the right permissions?

In many migrations there are changes to things like User Roles, Groups, and content location. All of these have an impact on Effective Permissions. Though it’s unlikely you will have the exact same Permission structure on Tableau Cloud that you had on Tableau Server, you can still verify Permissions in two ways.

- **Validating an individual content item**: The simplest way to validate individual asset permissions is to navigate to the relevant content item in the UI. Select the ellipsis next to an asset’s name, then select **Permissions**. You can see a detailed view of the effective permissions on an item. From here you can drill down to single Users or Groups, see what Permissions have been applied and make any desired changes.
- **Validating permissions on multiple content items or validating all permissions that a User or Group has**: To get an aggregate view on effective permissions, use the **Permissions** data source in [Admin Insights](https://help.tableau.com/current/online/en-us/adminview_insights.htm). Use this data source to build out a custom workbook that helps you validate effective permissions in the way that is most relevant to your migration. The following is an example view that could be built out with this data to assess effective permissions. **Note**: Admin Insights data sources update daily or weekly depending on your Settings configurations. You must wait until the data updates to get the most up-to-date data on Viz Load Times. You must wait until the data updates to get the most up-to-date view on your permissions. You will also not be able to make changes to permissions from this data source. To make a permissions change, navigate to the content item and make the change there.

### Is the data within the dashboard the same?

Tableau Cloud does not support automated data validation testing in order to manage load on the multi-tenant environment. To confirm assets are working as intended, we recommend that you review a sample set of assets manually to validate content has migrated as expected. The following is the recommended approach to do so.

1. Open a view on Tableau Cloud.
2. Open the same view on Tableau Server.
3. Compare the values manually across the two views. If a view relies on a data source with Row Level Security, you must add that Row Level Security to the data source on Tableau Cloud for the data to match. See the [End-User Migration Checklist](https://help.tableau.com/current/guides/migration/en-us/cloud_migration_part7.htm) for more detail.

Partners like [Wiiisdom](https://wiiisdom.com/wiiisdom-ops/tableau/) have also developed solutions that assist in data validation. For a full list of experienced migration partners, see [Experienced Tableau Cloud Migration Partners](https://www.tableau.com/solutions/tableau-migration#partners).

### Are my dashboards performing well?

Tableau Cloud does not support automated performance validation testing in order to manage load on the multi-tenant environment. To understand performance benchmarks, you can use the Viz Load Times data source in [Admin Insights](https://help.tableau.com/current/online/en-us/adminview_insights.htm). Use this data source to build a custom workbook so that you can understand metrics for the content on your Tableau Cloud site. These metrics could be Average Load Time, Medium Load Time, and P95 Load Time.

## Step 2: Recreate or manually migrate all items that aren’t programmatically migrated to Tableau Cloud.

Credentials embedded in a data source connection, content stored in Personal Spaces, Favorites, and User settings must be migrated by end users after they’re onboarded. For more information, see Data not supported by the Migration SDK.

## Step 3: Onboard end users

If you choose to supply adjusted usernames to Tableau Cloud to avoid the site invite email notification, adjust usernames back to the correct email address at this point. Refer to the steps in the [End-User Migration Checklist](https://help.tableau.com/current/guides/migration/en-us/cloud_migration_part7.htm) to help your users migrate any remaining items.

---

# Best Practices

Before your migration to Tableau Cloud, we recommend that you follow the steps in this topic to ensure best practices in both the migration itself and in using Tableau Cloud long term.

- Use the [Tableau Cloud Migration Technical Readiness Assessment Accelerator](https://exchange.tableau.com/products/921) to determine if you’re a good fit for a Cloud migration.
- [Remove Stale Content](https://help.tableau.com/current/server/en-us/adminview-stale-content.htm) (delete if older than N days w/o access) before migration.
- Centralize on [Published Data Sources](https://help.tableau.com/current/pro/desktop/en-us/publish_datasources_about.htm).
- Set Permissions at the Group and Project level and [lock Project Permissions](https://help.tableau.com/current/server/en-us/permissions_projects.htm)
- Move local files to a Cloud-accessible location to avoid using Bridge resources for local files.
- Extract all .csv/.excel files. Tableau Cloud doesn’t support live connections to local files. Bridge is required to connect to those local files.
- Review and understand your environment using [Admin Insights](https://help.tableau.com/current/online/en-us/adminview_insights.htm).
- Review Tableau cloud storage limitations. Certain Tableau content types have storage capacity limitations on Tableau Cloud. For more information about these limitations, see [Tableau Cloud Site Capacity](https://help.tableau.com/current/online/en-us/to_site_capacity.htm).
- Allocate enough free space for a successful migration. Encrypted extracts require three times their size on Tableau Server for encryption and decryption processes.

---

# Troubleshooting

If you have trouble with your migration using the Migration SDK, see the [Troubleshooting](https://tableau.github.io/migration-sdk/articles/troubleshooting.html) article in the API documentation.

---

# What's New in the Migration SDK

- What's New

- [](http://tableau.com)

- What's New
- Understanding the Migration SDK
- Migration SDK Overview
- Concepts
- Supported Content Types
- Preparing your migration
- Plan your Migration
- Pre-Migration Checklist
- Install the Migration SDK
- Design your Application
- Best Practices
- Migrating to Tableau Cloud
- Run Your Migration
- Complete Post-Migration Tasks
- Troubleshooting
- Reference
- Architecture
- [API Reference](https://tableau.github.io/migration-sdk/)
- About Tableau Help

# What's New

For release information, see [Tableau Migration SDK Releases](https://github.com/tableau/tableau-migration-sdk/releases).

---

## Articles (Tutorials and Examples)

---

# Data Loading

Migration SDK loads data during migration for purposes such as:

- Finding content items to migrate.
- Mapping references between content items (e.g. ownership, permissions, etc.) to valid destination items.
- Converting Tableau Server schedules to Tableau Cloud schedules.

To efficiently handle migrations of large sites, the default data loading behavior loads all items, using the batch size for paging, and caches necessary information for future use. In most cases this "bulk loading" reduces total API calls compared to loading items individually, which minimizes API throttling and overall migration time.

For migration of large sites where only a few items are necessary, however, the time spent loading all items may exceed the time savings of reduced API calls. In this situation data loading can be configured through the plan builder.

- If a content type with many items has already migrated but few are actually referenced, for example when user migration has already been completed, skipping the content type with pre-caching disabled is recommended.
- In advanced scenarios where custom logic is needed, migration services can be registered to control data loading behavior.

## Migration Content Loader

A migration content loader is responsible for finding the content items from the source site that should be considered for migration. Loaders produce batches of content items that are then mapped and filtered before migration. Items returned by the loader are added to the manifest and used when finding content references on the source site.

Custom migration content loaders can be used by overriding the `IMigrationContentLoader` migration service, either for all content types or a specific content type. See the Custom Migration Services topic for details on overriding migration services.

- Python
- C#

```
from typing import TypeVar
from tableau_migration import (
    empty_pager,
    MigrationContentLoaderBase
)

TContent = TypeVar("T")

class EmptyMigrationContentLoader(MigrationContentLoaderBase[TContent]):
    def get_migration_content_pager(self, page_size: int):
        return empty_pager(TContent)
```

```
public class EmptyMigrationContentLoader<TContent> : IMigrationContentLoader<TContent>
    where TContent : IContentReference
{
    public IPager<TContent> GetMigrationContentPager(int pageSize)
        => new MemoryPager<TContent>([], pageSize);
}
```

## Content Reference Loading

When updating content references (e.g. a projects's owner ID), data from both source and destination sites are loaded as necessary. Content reference data found through the normal course of the migration are cached automatically for use in future content types, but when content items are referenced that were filtered out or otherwise excluded from migration, additional data load attempts are made by the cache before producing migration errors.

Custom content reference loading behavior can be used by overridding the `IContentReferenceCacheLoadStrategyProvider` migration service, either for all content types or a specific content type. Built-in providers are are available for the default bulk loading and lazy loading are available. See the Custom Migration Services topic for details on overriding migration services.

- Python
- C#

```
plan_builder.services.set(ContentReferenceCacheLoadStrategyProviderBase[IUser], LazyContentReferenceCacheLoadStrategyProvider[IUser])
```

```
planBuilder.Services.Set<IContentReferenceCacheLoadStrategyProvider<IUser>>(MigrationServiceFactoryContext ctx =>
{
	return ctx.Services.GetRequiredService<LazyContentReferenceCacheLoadStrategyProvider<IUser>>();
});
```

---

# Configuration

The Migration SDK uses two sources of configuration

1. Basic Configuration that uses the Migration Plan. It contains configuration for a specific migration run.
2. Advanced Configuration. This configuration that is unlikely to change between migration runs.

## Basic Configuration

The bare minimum Migration SDK configuration is done using a Migration Plan. It defines the source, destination, and hooks executed during migration. The easiest way to generate a new Migration Plan is using `MigrationPlanBuilder`(`IMigrationPlanBuilder` implementation). Before you build a new plan, you need to:

- Define Source.
- Define Destination.
- Define the Migration Type.
- Customize with hooks (optional).

##### Important

Personal access tokens (PATs) are long-lived authentication tokens that allow you to sign in to the Tableau REST API without requiring hard-coded credentials or interactive sign-in. Best practices

- Revoke and generate a new PAT every day to keep your server secure.
- Access tokens should not be stored in plain text in application configuration files. Instead, use secure alternatives, such as encryption or a secrets management system.
- If the source and destination sites are on the same server, use separate PATs.

### Source

The method `MigrationPlanBuilder.FromSourceTableauServer` defines the source server by instantiating a new `TableauSiteConnectionConfiguration` with the following parameters:

- serverUrl
- siteContentUrl (optional)
- accessTokenName
- accessToken

### Destination

The method `MigrationPlanBuilder.ToDestinationTableauCloud` defines the destination server by instantiating a new `TableauSiteConnectionConfiguration` with the following parameters:

- podUrl
- siteContentUrl: This is the site name on Tableau Cloud.
- accessTokenName
- accessToken

### Migration Type

The method `MigrationPlanBuilder.ForServerToCloud` defines the migration type and load all default hooks for a **Server to Cloud** migration.

### Add Hooks (optional)

The Plan Builder exposes the properties `MigrationPlanBuilder.Hooks`, `MigrationPlanBuilder.Filters`, `MigrationPlanBuilder.Mappings`, and `MigrationPlanBuilder.Transformers`. With these properties, you can customize your migration plan. See Custom Hooks article for more details.

### Build

The method `MigrationPlanBuilder.Build` generates a Migration Plan ready to be used as an input to a migration process.

## Advanced configuration

`MigrationSdkOptions` is the configuration class the Migration SDK uses internally to process a migration. It contains adjustable properties that change migration engine behavior. These properties are useful tools to troubleshoot and tune the migration process.

Advanced configuration uses the [.NET Configuration Framework](https://learn.microsoft.com/en-us/dotnet/core/extensions/configuration) to build `MigrationSdkOptions` from a set of configuration providers.

##### Note

Unless specified otherwise, all configuration options are dynamically applied for configuration providers that support change detection.

- Python
- C#

When using Python, Migration SDK configuration values can be set via environment variables. Environment variables are evaluated at the time the `tableau_migration` module is imported. To reload Migration SDK configuration after modifying environment variables during runtime, call the reload_configuration function.

Environment variables do not support the `:` hierarchy delimiter used by other configuration providers. The double underscore (`__`) delimiter is used instead. All configuration environment variables start with `MigrationSDK__`.

We recommend using a [.NET Generic Host](https://learn.microsoft.com/en-us/dotnet/core/extensions/generic-host?tabs=appbuilder) to initialize the application. This will enable setting configuration values via `appsettings.json` which can be passed into `userOptions` in `.AddTableauMigrationSdk`. See .NET getting started examples for more info.

### ContentTypes

This is an array of `MigrationSdkOptions.ContentTypesOptions`. Each array object corresponds to settings for a single content type.

##### Important

The type values are case-insensitive. Duplicate type key values will result in an exception.

- Python
- C#

#### Python Environment Variables

- `MigrationSDK__ContentTypes__<array index>__<content type config key>__Type`.
- `MigrationSDK__ContentTypes__<array index>__<content type config key>__BatchSize`.

**Example:** To set the `User` `BatchSize` to `201` and `Project` BatchSize to `203`, you would set environment variables as follows. Note the array indexes. They tie the setting values together in the Migration SDK.

```
# User BatchSize is 201
MigrationSDK__ContentTypes__0__Type = User
MigrationSDK__ContentTypes__0__BatchSize = 201

# Project BatchSize is 203
MigrationSDK__ContentTypes__1__Type = Project
MigrationSDK__ContentTypes__1__BatchSize = 203
```

In the following `json` example config file,

- A `BatchSize` of `201` is applied to the content type `User`.
- A `BatchSize` of `203` for `Project`.
- A `BatchSize` of `200` for `ServerExtractRefreshTask`.

```
{
    "MigrationSdkOptions": {
        "contentType": [
        {
            "type":"User",
            "batchSize": 201
        },
        {
            "type":"Project",
            "batchSize": 203
        },
        {
            "type":"ServerExtractRefreshTask",
            "batchSize": 200
        }
        ],
    }
}
```

The following table describes each setting. They should always be set per content type as described previously. If a setting below is not set for a content type, the Migration SDK falls back to the default value.

| Key | Description | Default | Python Environment Variable |
|---|---|---|---|
| `ContentTypes.Type` | Determines which content type the settings apply to. Only supported content types will be considered and all others will be ignored. This key comes from the interface for the content type. For example, the key for IUser is 'User'. Content type values are case insensitive. | Not applicable. | `MigrationSDK__ContentTypes__<array-index>__<type-key>__Type` |
| `ContentTypes.BatchSize` | Defines the page size of each list request. See [ Tableau REST API Paginating Results ](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_concepts_paging.htm) for details. | `100` | `MigrationSDK__ContentTypes__<array-index>__<type-key>__BatchSize` |
| `ContentTypes.BatchPublishingEnabled` | Selects the mode to publish a given content type. **Important:** This option is available only for Users. | `false` | `MigrationSDK__ContentTypes__<array-index>__<type-key>__BatchPublishingEnabled` |
| `ContentTypes.IncludeExtractEnabled` | **DEPRECATED:** This option is not supported and datasource publishing with extract is required. | `` | `MigrationSDK__ContentTypes__<array-index>__<type-key>__IncludeExtractEnabled` |
| `ContentTypes.OverwriteGroupUsersEnabled` | Determines whether to overwrite group users. Default: enabled. **Important:** This option is available only for Groups. | `true` | `MigrationSDK__ContentTypes__<array-index>__<type-key>__OverwriteGroupUsersEnabled` |
| `ContentTypes.OverwriteUserFavoritesEnabled` | Determines whether to overwrite user favorites. Default: enabled. **Important:** This option is available only for Favorites. | `true` | `MigrationSDK__ContentTypes__<array-index>__<type-key>__OverwriteGroupUsersEnabled` |
| `ContentTypes.MaxContentSize` | The maximum content size in bytes. Default: null. **Important:** This option is available only for DataSource, Workbook, and Flow content types. | `null` | `MigrationSDK__ContentTypes__<array-index>__<type-key>__MaxContentSize` |

### MigrationParallelism

This setting defines the number of parallel tasks migrating the same type of content simultaneously. You can tune the Migration SDK processing time with this configuration.

*Default:* 10 *Python Environment Variable:* `MigrationSDK__MigrationParallelism`

##### Warning

There are [concurrency limits in REST APIs on Tableau Cloud](https://kb.tableau.com/articles/issue/concurrency-limits-in-rest-apis-on-tableau-cloud). The current default configuration is the balance between performance without blocking too many resources to the migration process.

### File

This section contains options related to file storage.

| Key | Description | Default | Python Environment Variable |
|---|---|---|---|
| `Files.DisableFileEncryption` | Defines whether to encrypt temporary files downloaded during the migration. This applies to file-based content types, such as Workbooks and Data Sources. | `false` | `MigrationSDK__Files__DisableFileEncryption` |
| `Files.RootPath` | Defines the location to store temporary files. | ` The default temporary path for the OS.` | `MigrationSDK__Files__RootPath` |

### Network

This configuration section contains network-related options.

##### Important

`NetworkOptions.UserAgentComment` is not dynamically applied. It takes effect when you restart your application.

| Key | Description | Default | Python Environment Variable |
|---|---|---|---|
| `Network.FileChunkSizeKB` | The chunk size (KB) for publishing large files. This applies to file-based content types like Workbooks and Data Sources. See the [ Tableau REST API Publishing Resources documentation ](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_concepts_publish.htm) for more details. | `65536` | `MigrationSDK__Network__FileChunkSizeKB` |
| `Network.RequestsLoggingEnabled` | Enables logging of HTTP request start events. | `true` | `MigrationSDK__Network__RequestsLoggingEnabled` |
| `Network.HeadersLoggingEnabled` | Enables logging of HTTP request headers. | `false` | `MigrationSDK__Network__HeadersLoggingEnabled` |
| `Network.ContentLoggingEnabled` | Enables logging of HTTP request content. | `false` | `MigrationSDK__Network__ContentLoggingEnabled` |
| `Network.BinaryContentLoggingEnabled` | Enables logging of binary content in HTTP requests. | `false` | `MigrationSDK__Network__BinaryContentLoggingEnabled` |
| `Network.WorkbookContentLoggingEnabled` | Indicates whether the SDK logs workbook content when downloading a .twb workbook. | `false` | `MigrationSDK__Network_WorkbookContentLoggingEnabled` |
| `Network.ExceptionsLoggingEnabled` | Enables logging of HTTP request exceptions. | `false` | `MigrationSDK__Network__ExceptionsLoggingEnabled` |
| `Network.UserAgentComment` | Defines a comment to append to the User-Agent header in all HTTP requests. This property is only used to assist in server-side debugging and it not typically set. |  | `MigrationSDK__Network__UserAgentComment` |

#### Resilience

The `Resilience` sub-section deals with the resilience and transient-fault layer. See [Microsoft.Extensions.Http.Resilience](https://learn.microsoft.com/en-us/dotnet/core/resilience) for more details.

| Key | Description | Default | Python Environment Variable |
|---|---|---|---|
| `Network.Resilience.RetryEnabled` | Defines whether to retry failed requests. | `true` | `MigrationSDK__Network__Resilience__RetryEnabled` |
| `Network.Resilience.RetryIntervals` | Defines the number of retries and interval between retries. | [ `500` ms, `500` ms, `500` ms, `1` s, `2` s ] | Not supported |
| `Network.Resilience.RetryOverrideResponseCodes` | Overrides the default error status codes for retries. |  | Not supported |
| `Network.Resilience.ConcurrentRequestsLimitEnabled` | Defines whether to limit concurrent requests. | `false` | `MigrationSDK__Network__Resilience__ConcurrentRequestsLimitEnabled` |
| `Network.Resilience.MaxConcurrentRequests` | Defines the maximum quantity of concurrent API requests. This is based on the [number of logical processors](https://learn.microsoft.com/en-us/dotnet/api/system.environment.processorcount#remarks) (or `processor count`). | (`processor count`)/`2` | `MigrationSDK__Network__Resilience__MaxConcurrentRequests` |
| `Network.Resilience.ConcurrentWaitingRequestsOnQueue` | Defines the quantity of concurrent API requests waiting on queue. This is also based on `processor count`. | (`processor count`)/`4` | `MigrationSDK__Network__Resilience__ConcurrentWaitingRequestsOnQueue` |
| `Network.Resilience.ClientThrottleEnabled` | Defines whether to limit requests to a given endpoint. | `false` | `MigrationSDK__Network__Resilience__ClientThrottleEnabled` |
| `Network.Resilience.MaxReadRequests` | Limits the amount of GET requests for the Client Throttle. | `40000` | `MigrationSDK__Network__Resilience__MaxReadRequests` |
| `Network.Resilience.MaxReadRequestsInterval` | Defines the interval for the Read Request Throttle. | `1` h | `MigrationSDK__Network__Resilience__MaxReadRequestsInterval` |
| `Network.Resilience.MaxPublishRequests` | Defines the maximum quantity of non-GET requests on the client side. | `5500` | `MigrationSDK__Network__Resilience__MaxPublishRequests` |
| `Network.Resilience.MaxPublishRequestsInterval` | Defines the interval for the limit of non-GET requests on the client side. | `1` day | `MigrationSDK__Network__Resilience__MaxPublishRequestsInterval` |
| `Network.Resilience.ServerThrottleEnabled` | Defines whether to retry requests throttled on the server. | `true` | `MigrationSDK__Network__Resilience__ServerThrottleEnabled` |
| `Network.Resilience.ServerThrottleLimitRetries` | Defines whether there is a limit of retries for a throttled request. | `false` | `MigrationSDK__Network__Resilience__ServerThrottleLimitRetries` |
| `Network.Resilience.ServerThrottleRetryIntervals` | Defines the interval between each retry for throttled requests without the 'Retry-After' header. | [ `1` s, `3` s, `10` s, `30` s, `1` m ] | Not supported |
| `Network.Resilience.PerRequestTimeout` | Defines the maximum duration of non-FileTransfer requests. | `30` m | `MigrationSDK__Network__Resilience__PerRequestTimeout` |
| `Network.Resilience.PerFileTransferRequestTimeout` | Defines the maximum duration of FileTransfer requests. | `12` h | `MigrationSDK__Network__Resilience__PerFileTransferRequestTimeout` |

### DefaultPermissionsContentType

| Key | Description | Default | Python Environment Variable |
|---|---|---|---|
| `DefaultPermissionsContentTypes.UrlSegments` | List of types of default permissions for a given project. See [Query Default Permissions](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_permissions.htm#query_default_permissions) for details. **Important:** This configuration is not dynamically applied. It takes effect when you restart your application. | Listed in `DefaultPermissionsContentTypeUrlSegments` | Not Supported |

### Job

The Migration SDK uses two methods to publish the content to a destination server:

1. 'Bulk process': A single REST API call for multiple items.
2. 'Individual process': One REST API call per item.

This configuration only applies to the 'Bulk process'. Each batch publish REST API call returns a Job ID (see the [Tableau REST API Query Job](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_jobs_tasks_and_schedules.htm#query_job) for details). The SDK uses this ID to determine job status. The following table describes the related settings.

| Key | Description | Default | Python Environment Variable |
|---|---|---|---|
| `Jobs.JobPollRate` | Defines the interval to wait to recheck processing status for bulk processes. | `3` s | `MigrationSDK__Jobs__JobPollRate` |
| `Jobs.JobTimeout` | Defines the maximum interval to wait for a job to complete for bulk processes. | `30` m | `MigrationSDK__Jobs__JobTimeout` |

### Preflight

| Key | Description | Default | Python Environment Variable |
|---|---|---|---|
| `PreflightOptions.ValidateSettings` | Determines whether to validate supported site settings during the preflight step. | `true` | `MigrationSDK__Network__ValidateSettings` |

---

# Skipping Content Types

Migration SDK allows skipping all items of a content type through use of the migration plan builder. When one or more items of a content type should migrate, filters should be used, instead.

- Python
- C#

```
plan_builder.skip_content_type(IUser)
```

```
planBuilder.SkipContentType<IUser>();
```

## Pre-Caching

The pre-caching option controls the data loading behavior of the content type, which impacts how data is retrieved when items of other content types reference the skipped content type. For most migrations the default value (enabled) is recommended, as it reduces the total number of API calls to minimize API throttling regardless of how often the skipped content type is referenced.

### Pre-Caching Enabled

When pre-caching is enabled (default), this is equivalent to registering a filter for the content type that skips all items. The default data loading behavior is used.

### Pre-Caching Disabled

When pre-caching is disabled, a filter is registered to skip all items, but additional changes to data loading are configured for the content type. Disabling pre-caching is recommended only when the skipped content type has a very large number of items, and only a few of those items are referenced by the rest of the migrating content types. When pre-caching is disabled:

- A content loader is registered for the content type that returns zero items.
- The content reference cache load strategy for the content type is configured to search for items individually when possible.

---

# Features and Tableau REST API Versions

The following table shows all content types supported by the Tableau Migration SDK and their corresponding REST API versions. This helps you determine which features are available based on your Tableau Server or Tableau Cloud version.

| Content Type or Feature | First Introduced | REST API Version(s) | Tableau Version(s) | Comments |
|---|---|---|---|---|
| Users | SDK 1.0.0 | 2.0+ | 2018.1+ | [Tableau REST API: Get Users on Site](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_users_and_groups.htm#get_users_on_site) |
| Groups | SDK 1.0.0 | 2.0+ | 2018.1+ | [Tableau REST API: Get Groups for a Site](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_users_and_groups.htm#query_groups) |
| Projects | SDK 1.0.0 | 2.0+ | 2018.1+ | [Tableau REST API: Query Projects](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_projects.htm#query_projects) |
| Data Sources | SDK 1.0.0 | 2.0+ | 2018.1+ | [Tableau REST API: Query Data Sources](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_data_sources.htm#query_data_sources) |
| Workbooks | SDK 1.0.0 | 2.0+ | 2018.1+ | [Tableau REST API: Query Workbooks for Site](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_workbooks_and_views.htm#query_workbooks_for_site) |
| Flows | SDK 6.1.0 | 3.22+ | 2024.2+ | [Tableau REST API: Publish Flow](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_flow.htm#publish_flow) |
| Permissions | SDK 1.0.0 | 2.0+ | 2018.1+ | [Tableau REST API: Permissions Methods](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_permissions.htm#query_permissions_for_workbook) |
| Extract Refresh Tasks | SDK 4.1.0 | 3.19/3.20 | 2023.1/2023.2 | [Tableau REST API: Get Extract Refresh Tasks](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_extract_and_encryption.htm#query_extract_refresh_tasks) |
| Custom Views | SDK 4.3.0 | 3.21 | 2023.3 | [Tableau REST API: List Users with Custom View as Default](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_workbooks_and_views.htm#list_users_with_custom_view_as_default) |
| Subscriptions | SDK 5.1.1 | 3.22/3.23 | 2024.1/2024.2 | [Tableau REST API: List Subscriptions](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_subscriptions.htm#list_subscriptions) |
| Embedded Credentials | SDK 5.1.1 | 3.22/3.23 | 2024.1/2024.2 | Server 2024.2+ only [Tableau REST API: Download Data Source Encrypted Keychain](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_data_sources.htm#download_datasource_encrypted_keychain) [Tableau REST API: Download Workbook Encrypted Keychain](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_workbooks_and_views.htm#download_workbook_encrypted_keychain) |
| Multi-IdP Support | SDK 5.1.1 | 3.24/3.25 | 2024.3/2025.1 | [Tableau REST API: List Authentication Configurations in a Site](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_site.htm#list_authentication_configurations_site) [Tableau REST API: Add User to Site](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_users_and_groups.htm#add_user_to_site) |
| Group Sets | SDK 5.3.0 | 3.22 | 2024.2 | [Tableau REST API: List Group Sets](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_users_and_groups.htm#list_group_sets) |
| Favorites | SDK 5.3.0 | 3.23 | 2024.2 | [Tableau REST API: Get Favorites for User](https://help.tableau.com/current/api/rest_api/en-us/REST/rest_api_ref_favorites.htm#query_favorites) |

---

# Anatomy of a Custom View File

The Migration SDK downloads Custom View definition files during the migration process. There files are in JSON format. This is what a Custom View File looks like:

```
[
    .....
    {
        "isSourceView": true,
        "viewName": "View 1",
        "tcv": "{base-64 custom view xml content with + signs replaced by -}"
    }
    .....
]
```

## Getting the Custom View Definition from the Custom View File

In the JSON file, the field `tcv` contains the XML definition of a custom view. This field is encoded as a Base64 string, with `+` characters replaced by `-`.

To decode the `tcv` value

1. Replace all the `-` characters in the string with `+`.
2. Decode the resulting Base64 string.
3. Get the XML Custom View Definition.

To encode the `tcv` value

1. Make any changes to the Custom View Definition.
2. Encode the XML into a Base64 string.
3. Replace all the `+` characters in the string with `-`.

---

# Dependency Injection

Migration SDK uses [.NET Dependency Injection](https://learn.microsoft.com/en-us/dotnet/core/extensions/dependency-injection) (DI) to manage required service dependencies.

## Dependency Retrieval and Management

The entry point for application DI is the [IServiceProvider](https://learn.microsoft.com/en-us/dotnet/api/system.iserviceprovider?view=net-6.0) interface. This acts as a container to retrieve registered services and manage their lifetimes.

Services that are configured on a per-plan or per-endpoint basis can be managed through custom migration services.

## Dependency Lifetimes

Service lifetimes are controlled by the [ServiceLifetime](https://learn.microsoft.com/en-us/dotnet/api/microsoft.extensions.dependencyinjection.servicelifetime?view=dotnet-plat-ext-6.0) enum. These are automatically applied using `IServiceCollection.AddSingleton/AddScoped/AddTransient` extension methods.

## Dependency Registration

Individual services are registered using an [IServiceCollection](https://learn.microsoft.com/en-us/dotnet/api/microsoft.extensions.dependencyinjection.iservicecollection?view=dotnet-plat-ext-6.0) instance. This instance is a collection of registered services that is built to create an [IServiceProvider](https://learn.microsoft.com/en-us/dotnet/api/system.iserviceprovider?view=net-6.0) instance for service retrieval and management.

## Migration SDK Dependency Injection

IServiceCollectionExtensions.AddTableauMigrationSdk is the main method used to register default Migration SDK services. This call is required to set up the dependencies required by the SDK.

Scoped services are used to isolate services, for example for source and destination API clients.

## Sample Code

The sample below shows a console application that initializes the Migration SDK using dependency injection. The full code is available in the `DependencyInjection.ExampleApplication` project.

```
using System;
using System.Collections.Generic;
using System.Threading.Tasks;
using DependencyInjection.ExampleApplication.Hooks.Filters;
using DependencyInjection.ExampleApplication.Hooks.Mappings;
using Microsoft.Extensions.DependencyInjection;
using Tableau.Migration;

namespace DependencyInjection.ExampleApplication
{
    public static class Program
    {
        // Update the values here to filter the service output.
        private static readonly IEnumerable<Type>? DisplayFilter = null; // new[] { typeof(ProjectsFilter), typeof(ProjectMapping) };

        public static async Task Main()
        {
            // Initialize a new service collection
            var serviceCollection = new ServiceCollection()

                // Register the default migration-related services.
                .AddTableauMigrationSdk()

                // Set up logging to redirect to the console.
                .AddConsoleLogging()

                // Register a singleton service.
                .AddSingleton<SingletonService>()

                // Register a scoped filter.
                .AddScoped<ProjectsFilter>()

                /// Register a scoped mapping.
                .AddScoped<ProjectMapping>()

                // Display the services in the collection.
                .DisplayServices(DisplayFilter);

            // Build the service provider (container) instance to manage services.
            // Dependencies will be retrieved from this instance.
            await using var serviceProvider = serviceCollection.BuildServiceProvider();

            // Verify the services are configured correctly.
            await VerifyServicesAsync(serviceProvider);
        }

        private static async Task VerifyServicesAsync(IServiceProvider serviceProvider)
        {
            // Create a scope to access our scoped services.
            await using var scope1 = serviceProvider.CreateAsyncScope();

            // Retrieve some services from our scoped provider...
            var scope1Singleton = scope1.ServiceProvider.GetRequiredService<SingletonService>();

            var scope1Filter1 = scope1.ServiceProvider.GetRequiredService<ProjectsFilter>();
            var scope1Mapping1 = scope1.ServiceProvider.GetRequiredService<ProjectMapping>();

            var scope1Filter2 = scope1.ServiceProvider.GetRequiredService<ProjectsFilter>();
            var scope1Mapping2 = scope1.ServiceProvider.GetRequiredService<ProjectMapping>();

            // Create another scope to access our scoped services.
            await using var scope2 = serviceProvider.CreateAsyncScope();

            // Retrieve some services from our scoped provider...
            var scope2Singleton = scope1.ServiceProvider.GetRequiredService<SingletonService>();

            var scope2Filter = scope2.ServiceProvider.GetRequiredService<ProjectsFilter>();
            var scope2Mapping = scope2.ServiceProvider.GetRequiredService<ProjectMapping>();

            // Because the SingletonService was registered as a singleton,
            // requests across scopes will return the same instance.
            Assert.SameReferences(scope1Singleton, scope2Singleton);

            // Because we registered these services as scoped, other requests for the service
            // within the same scope will return the initial instance.
            Assert.SameReferences(scope1Filter1, scope1Filter2);
            Assert.SameReferences(scope1Mapping1, scope1Mapping2);

            // Because we are accessing services from a different scope, the instances
            // returned here will be different from the first scope.
            Assert.DifferentReferences(scope1Filter1, scope2Filter);
            Assert.DifferentReferences(scope1Mapping1, scope2Mapping);
        }
    }
}
```

---

# Custom Migration Services

Migration services provide an alternative path for dependency injection to customize Migration SDK behavior. Replacing most DI services is controlled through the application's `IServiceProvider` container, which typically does not change after application startup and is not easily available in all contexts. Migration services are those DI services that the Migration SDK obtains through the migration plan, which may or may not ultimately come from the `IServiceProvider` container. This allows migration services to be customized on a per-plan basis, and makes service customization easier in interoperability scenarios (e.g. Python).

## Supported Migration Services

The services available to override through the migration services feature are listed in the plan builder's service collection. If the migration service is generic the open generic type is listed in the supported services list.

- Python
- C#

Supported migration services are available through the plan builder's `services` property.

```
plan_builder = MigrationPlanBuilder()
for service in plan_builder.services.supported_services:
	print(service.name) # Service name contains the
```

Supported migration services are available through the plan builder's `Services` property.

```
var planBuilder = new MigrationPlanBuilder();
foreach(var service in planBuilder.Services.SupportedServices)
{
    Console.WriteLine(service.Name);
}
```

## Overriding Migration Services

All migration services have default implementations provided by the Migration SDK. When a migration service is registered with the plan builder it will be used in place of the default implementation for that plan.

- Python
- C#

Override migration services on a per-plan basis through the plan builder's `services` property.

#### Migration Service Class

Like hooks, migration services are created by inheriting from a service base class.

```
from typing import TypeVar
from tableau_migration import (
    empty_pager,
    MigrationContentLoaderBase
)

TContent = TypeVar("T")

class EmptyMigrationContentLoader(MigrationContentLoaderBase[TContent]):
    def get_migration_content_pager(self, page_size: int):
        return empty_pager(TContent)
```

#### Registration

Migration services are then registered with the service builder for a given service type.

```
plan_builder.services.set(MigrationContentLoaderBase[IUser], EmptyMigrationContentLoader[IUser])
```

Override migration services on a per-plan basis through the plan builder's `Services` property.

#### Migration Service Class

Like hooks, migration services are created by implementing a service interface.

```
public class EmptyMigrationContentLoader<TContent> : IMigrationContentLoader<TContent>
    where TContent : IContentReference
{
    public IPager<TContent> GetMigrationContentPager(int pageSize)
        => new MemoryPager<TContent>([], pageSize);
}
```

#### Registration

A service factory is then registered with the service builder for a given service type. The service factory context includes the scoped `IServiceProvider` container for types available through application DI.

```
planBuilder.Services.Set<IMigrationContentLoader<IUser>>(MigrationServiceFactoryContext ctx =>
{
	return ctx.Services.GetRequiredService<EmptyMigrationContentLoader<IUser>>();
});
```

## Generic Migration Services

Many migration services are [generic](https://learn.microsoft.com/en-us/dotnet/standard/generics), meaning they have type arguments. Normally these type arguments represent migrating content types, so that different migration service implementations can be used for different content types.

When a migration service overrides a *closed generic* type, meaning all type arguments are specified, that override will only be used for those type arguments.

Alternatively, a migration service can override the *open generic* type, meaning no type arguments are specified. Migration service overrides for open generic types are used when no other override is registered for the specific type arguments involved.

- Python
- C#

```
# Closed generic, only overrides IUser.
plan_builder.services.set(MigrationContentLoaderBase[IUser], EmptyMigrationContentLoader[IUser])

# Open generic, used for all types without a closed generic override.
plan_builder.services.set(MigrationContentLoaderBase, EmptyMigrationContentLoader)
```

The service factory context includes the requested type arguments for service creation.

```
// Closed generic, only overrides IUser.
planBuilder.Services.Set<IMigrationContentLoader<IUser>>(MigrationServiceFactoryContext ctx =>
{
	return ctx.Services.GetRequiredService<EmptyMigrationContentLoader<IUser>>();
});

// Open generic, used for all types without a closed generic override.
planBuilder.Services.Set(typeof(IMigrationContentLoader<>), MigrationServiceFactoryContext ctx =>
{
	return ctx.Services.GetRequiredService(typeof(EmptyMigrationContentLoader<>).MakeGenericType(ctx.Type.GetGenericArguments()));
});
```

---

# Custom Hooks

## Important Classes

Here are some important things to know when writing custom hooks:

- **Interfaces:** These interfaces expose supported methods. You can implement these directly or inherit available base classes. This applies to C# only.
- **Base Classes:** These classes can be inherited, allowing you to write your implementation in the overridden methods. You do not need to implement the interface explicitly if you use these.
- **Code Samples:** We have provided some simple code samples. You can use these as a starting point for your hooks.

- Python
- C#

The base classes can be used as they are linked in the API reference. However, for ease of use, all base classes have been imported into the `tableau_migration` namespace without the `Py` prefix. For example: `PyContentFilterBase` has been imported as `tableau_migration.ContentFilterBase`.

#### Pre-Migration

| Type | Base Class | Code Samples |
|---|---|---|
| Initialize Migration | `InitializeMigrationHookBase` | Code Samples/Initialize Migration |
| Filters | `ContentFilterBase[TContent]` | Code Samples/Filters |
| Mappings | `ContentMappingBase[TContent]` | Code Samples/Mappings |
| Pulled | `ContentItemPulledHookBase[TPrepare]` | Code Samples/Pulled |
| Transformers | `ContentTransformerBase[TPublish]` | Code Samples/Transformers |

#### Post-Migration

| Type | Base Class | Code Samples |
|---|---|---|
| Post-Publish | `ContentItemPostPublishHookBase[TPublish, TResult]` | Code Samples/Post-Publish Hooks |
| Bulk Post-Publish | `BulkPostPublishHookBase[TSource]` | Code Samples/Bulk Post-Publish |
| Batch Migration Completed | `ContentBatchMigrationCompletedHookBase[TContent]` | Code Samples/Batch Completed |
| Migration Action Completed | `MigrationActionCompletedHookBase` | Code Samples/Action Completed |

#### Registration

To register Python hooks, register the object with the appropriate hook type list in the plan builder.

#### Pre-Migration

| Type | Base Class | Interface | Code Samples |
|---|---|---|---|
| Initialize Migration | None | `IInitializeMigrationHook` | Code Samples/Initialize Migration |
| Filters | `ContentFilterBase<TContent>` | `IContentFilter<TContent>` | Code Samples/Filters |
| Mappings | `ContentMappingBase<TContent>` | `IContentMapping<TContent>` | Code Samples/Mappings |
| Pulled | `ContentItemPulledHookBase<TPrepare>` | `IContentItemPulledHook<TPrepare>` | Code Samples/Pulled |
| Transformers | `ContentTransformerBase<TPublish>` | `IContentTransformer<TPublish>` | Code Samples/Transformers |

#### Post-Migration

| Type | Base Class | Interface | Code Samples |
|---|---|---|---|
| Post-Publish | `ContentItemPostPublishHookBase<TPublish, TResult>` | `IContentItemPostPublishHook<TContent>` | Code Samples/Post-Publish Hooks |
| Bulk Post-Publish | `BulkPostPublishHookBase<TSource>` | `IBulkPostPublishHook<TSource>` | Code Samples/Bulk Post-Publish |
| Batch Migration Completed | None | `IContentBatchMigrationCompletedHook<TContent>` | Code Samples/Batch Completed |
| Migration Action Completed | None | `IMigrationActionCompletedHook` | Code Samples/Action Completed |

#### Registration

You can implement, register, and call hooks in the following ways:

1. **Object:** The caller supplies an object that implements a suitable interface. This is the most straightforward way to register. However, the caller must manage the object’s lifecycle and dependencies.
2. **Factory:** The caller supplies a type, with or without a factory function to create an object, that implements a suitable interface. This allows for the injection of SDK dependencies such as the manifest, content finders, etc. The lifecycle is managed by the DI container in the SDK (more details on .NET service lifetimes are [here](https://learn.microsoft.com/en-us/dotnet/core/extensions/dependency-injection#service-lifetimes)). Before adding the type, you must register the type on the DI container with the corresponding lifecycle.
3. **Callback:** The caller supplies a callback function that conforms to the `ExecuteAsync` method of the hook interface. This is essentially a functional version of #1. Internally, the SDK wraps the callback in a transient object to execute the function.

---

# Example Hook Use Cases

## Remove Unlicensed Users During Migration

First, create a filter for IUser items. The filter should check the LicenseLevel property for the `Unlicensed` value, either as a string constant or with the LicenseLevels.Unlicensed constant. This filter will prevent users matching the license level from being created on the destination site.

Additional logic is needed to handle references to these users since they will not exist on the destination site, for example if an unlicensed user owns a workbook. One method would be to create a mapping that maps unlicensed users to one or more alternate users that will exist on the destination site. This will allow default hooks to update user references to the alternate users.

The filter and mapping are then registered as custom hooks with the migration plan.

## Filter Out Content Under Certain Projects

To filter out a project and all content within it, create a project filter and cascade the filter. To filter only some content from a project, create a filter for data sources, workbooks, and other content types as desired. The filters should check the Location property to determine if the content is in a project that has been filtered. This can be done by comparing the path of the location's Parent method result to either a known set of project paths, or by looking up the project in the the migration manifest to see if the project was migrated.

The filters are then registered as a custom hook with the migration plan.

## Rename a Project

Content items are renamed through mappings so that references to content items can also be mapped during migration. To rename a project, create a mapping for IProject items. The mapping should change the final path segment of the project's ContentLocation as desired. Manipulating the path's last segment can be easily done through the location's Rename method.

The mapping is then registered as a custom hook with the migration plan.

## Combine Projects

To combine multiple source projects to a single destination project, a single destination project must be determined. The destination project can either be created before the migration, or a filter for IProject items can be created to filter all but one of the source projects. In the case of using a filter, a transformer can be created to merge permissions or other properties of the combined project when it is migrated.

With a single destination project determined a mapping for IProject items should be created. The mapping should map the source projects to the destination combined project's location.

The filters and mapping are then registered as custom hooks with the migration plan.

## Change a Published Data Source to Use Tableau Bridge

First, create a transformer for IPublishableDataSource items. The transformer should determine if the data source needs to use Tableau Bridge, and set the UseRemoteQueryAgent property to `true` for those data sources.

The transformer is then registered as a custom hook with the migration plan.

## Tagging Content During Migration

First, create a transformer for the desired content types that support tags. The transformer can then add a new Tag object to the Tags collection of the content item. A default post-publish hook will then ensure that the tag is added after the content item is published.

The transformer is then registered as a custom hook with the migration plan.

## Embed Connection Credentials

To embed credentials in connections, create a post-publish hook for the appropriate publish and result types. The hook should use dependeny injection to obtain the migration's destination endpoint. The APIs of the destination can be accessed through the SiteApi property of the endpoint after casting to the IMigrationApiEndpoint interface. The APIs of the desired content type should then be used to get the connection ID of the connection in question, and update the connection to embed the credentials.

The hook is then registered as a custom hook with the migration plan.

## Cancel Migration On Batch Failure

By default the migration SDK continues migration if a single content items fail to migrate. This allows for partial migration, but if fundamental items such a user fails to migrate, downstream items that reference this item may fail to migrate as well. Canceling the migration after failures are seen when a batch completes allows for faster intervention, while ensuring that items of the batch have completed and are not partially migrated.

First, create a batch completed hook for one or more content types. To inspect whether the batch migration has succeeded, check the ItemResults property of the context object. The status should not be the Error enumeration value. If an error is detected, the hook should return the value of the context object's ForNextBatch method with a `false` value for the `performNextBatch` argument.

The hook is then registered as a custom hook with the migration plan.

## Cancel Migration After Users and Groups

To prevent content types from migrating entirely a filter that filters out all items may be used, but this still lists the items on the source site. To end the migration early, an action completed hook can be created. The hook can use dependency injection to obtain the migration's manifest, and can inspect if the expected content types are returned by the GetPartitionTypes method of the manifest's Entries property. To cancel the migration, return the value of the context object's ForNextAction method with a `false` value for the `performNextAction` argument.

The hook is then registered as a custom hook with the migration plan.

---

# Filter Cascading

When applying filters, not migrating a content item can potentially effect other items that reference it. For example, if a user is not migrated, a workbook owned by that user would normally not be able to migrate without changing the owner or taking other steps to handle the reference. In many cases when a content item is skipped, the intention is to also skip migrating the content items that reference it. For example, if a project is not migrated, you might want to also skip the workbooks and data sources in that project. *Cascading* a filter allows it to apply to all content items that reference the item being filtered.

## Cascading Filters

To have a filter cascade to content items that reference the current item, assign a cascade skip status in a filter. Content items that reference this item will then have the same cascade skip status applied by a default filter when those content types are migrated.

- Python
- C#

```
item.status = FilterStatus.CASCADE_SKIP
```

```
item.Status = FilterStatus.CascadeSkip;
```

See Sample: Filter projects by name for a full example of a cascading filter.

## Non-Cascading Filters

To have a filter only apply to the current item, assign a non-cascade skip status in a filter. Content items that reference this item will need to be handled manually through additional filters, mappings, or transformers.

- Python
- C#

```
item.status = FilterStatus.SKIP
```

```
item.Status = FilterStatus.Skip;
```

See Sample: Filter users by site role for a full example of a non-cascading filter.

## Filter Ordering and Overriding

Filters run in the order they are registered. Each filter is run on all items of the content type available to filter, including those marked for exclusion by previous filters. This means that setting the filter status of an item overrides any decisions made by previous filters, including cascading filters from previous content types.

## Updating Boolean Filters

In previous versions of Migration SDK filters never cascaded, and used a simple boolean return value to determine whether the filtering content item was migrated or skipped. Filters using these boolean return values continue to function as before. To update these filters to allow them to cascade:

- Python
- C#

- Override the `filter` method instead of the `should_migrate` method.
- Convert the logic of the filter that returns `true` to either return without modifying the input context, or set the context's `status` property to `FilterStatus.MIGRATE` to overwrite previous filters.
- Convert the logic of the filter that returns `false` to set the context's `status` property either to `FilterStatus.SKIP` or `FilterStatus.CASCADE_SKIP`.

- Override the `Filter` method instead of the `ShouldMigrate` method.
- Convert the logic of the filter that returns `true` to either return without modifying the input context, or set the context's `status` property to `FilterStatus.Migrate` to overwrite previous filters.
- Convert the logic of the filter that returns `false` to set the context's `status` property either to `FilterStatus.Skip` or `FilterStatus.CascadeSkip`.

---

# Hooks

A hook is a means of modifying the standard functionality of the Migration SDK. These modifications include filtering, mapping, transforming migration content and reacting to other SDK events.

The Migration SDK has default hooks that run for every migration.

##### Note

You can also write custom hooks to fit your specific use cases.

## Types of Hooks

The Migration SDK has the following types of hooks, categorized broadly based on when they run.

### Content

These types of hooks run on content items.

- Filters: Used to exclude certain content items based on known criteria. Filters can optionally cascade to other content items that reference the skipped item.
- Mappings: Used to map a source item to something different at the destination. The original does not change.
- Transformers: Used to change certain properties within various content types. A good example is permissions where the source and destination have different identifiers. Important Transformers that change properties like names and IDs erase references to the original. If that is not intended, you should use mappings instead.

### General purpose

These types of hooks run before or after certain migration events.

- Migration Initialized: Executed after preflight validation is completed successfully, but before any migration actions are started.
- Pulled: Run on a source content item after the item has been fully retrieved and downloaded, but before it is converted and prepared for publishing.
- Post-Publish: Run on the destination content after the items for the content type have been published.
- Bulk Post-Publish: Executed after publishing a batch of content, when bulk publishing is supported. You can make changes to the published set of items with this type of hook. You can write this type of hook for content types such as Users.
- Migration Action Completed: Executed after the migration of each content type.
- Batch Migration Completed: Executed after the completion of the migration of a batch of Tableau’s content.

## Hook execution flow

This diagram displays how each hook is called as part of the migration process.

## Default Hooks

The plan builder may have pre-loaded hooks, called "Default" hooks. These are necessary due to inherent differences between Tableau Server and Tableau Cloud.

1. Default Filters
2. Default Mappings
3. Default Post-Publish Hooks
4. Default Transformers

---

# Updating Python Hooks from v3 to v4+

Python hooks received a major update in version 4 of the Tableau Migration SDK. In version 3, all Python hooks were thin wrappers around C# code, and the actual object manipulated by the hook was a C# object. In version 4 and beyond, everything is still a wrapper around C#, but the class itself and the context object passed to be manipulated are now fully in Python with no more C# required. This change brings full autocomplete support in your favorite Python IDE.

This is a breaking change and will require an update to existing v3 hooks. On this page, we'll go over how to update your v3 hooks to v4+.

See Custom Hooks to learn how to build v4 hooks.

## Filters

There are a few changes that must be made to existing filters for them to work in version 4+.

The imports have changed. All imports are now from the Python `tableau_migration` namespaces instead of the C# `Tableau.Migration` namespaces.

The `__namespace__` and `_dotnet_base` variables are no longer required.

The `ShouldMigration` function is now `should_migrate`.

For registration, the type is no longer required. Simply add the object to the filter list.

### Version 3 -> Version 4+ Filter Class Diff

```
-from Tableau.Migration.Content import IUser
-from Tableau.Migration.Engine.Hooks.Filters import ContentFilterBase
+from tableau_migration import (
+    ContentFilterBase,
+    ContentMigrationItem,
+    IUser
+)

class FilterBob(ContentFilterBase[IUser]):
    """A class to filter out all users named Bob."""

-    __namespace__ = "MyNamespace"
-    _dotnet_base = ContentFilterBase[IUser]

    def __init__(self):
        """Default init to set up logging."""
        self._logger = logging.getLogger(self.__class__.__name__)
        self._logger.setLevel(logging.DEBUG)

-    def ShouldMigrate(self,item):
+    def should_migrate(self, item: ContentMigrationItem[IUser]) -> bool:
        """Implements ShouldMigrate from base."""
-        if item.SourceItem.Name.casefold() == "Bob".casefold():
+        if item.source_item.name.casefold() == "Bob".casefold():
            self._logger.debug('%s filtered Bob', self.__class__.__name__)
            return False

        return True
```

### Version 3 -> Version 4+ Registration Diff

```
-   plan_builder.filters.add(IUser, FilterBob)
+   plan_builder.filters.add(FilterBob)
```

## Mappings

There are a few changes that must be made to existing mappings for them to work in version 4+.

The imports have changed. All imports are now from the Python `tableau_migration` namespaces instead of the C# `Tableau.Migration` namespaces.

The `__namespace__` and `_dotnet_base` variables are no longer required.

The `Execute` function is now `map` with the parameter being of type `ContentMappingContext[T]`.

For registration, the type is no longer required. Simply add the object to the mapping list.

### Version 3 -> Version 4+ Mapping Class Diff

```
-from Tableau.Migration.Interop.Hooks.Mappings import ISyncContentMapping
-from Tableau.Migration.Content import IUser
-from Tableau.Migration import ContentLocation
+from tableau_migration import (
+    ContentLocation,
+    ContentMappingBase,
+    ContentMappingContext,
+    IUser
+)

-class SpecialUserMapping(ISyncContentMapping[IUser]):
+class SpecialUserMapping(ContentMappingBase[IUser]):
    """A class to map users to server admin."""

-    __namespace__ = "MyNamespace"
-    _dotnet_base = ISyncContentMapping[IUser]
-    _admin_username = ContentLocation.ForUsername("domain", "admin")
+    _admin_username = ContentLocation.for_username("domain", "admin")

    def __init__(self):
        """Default init to set up logging."""
        self._logger = logging.getLogger(self.__class__.__name__)
        self._logger.setLevel(logging.DEBUG)

-    def Execute(self,ctx):  # noqa: N802
+    def map(self, ctx: ContentMappingContext[IUser]) -> ContentMappingContext[IUser]:
-       if ctx.ContentItem.Email.casefold() == "bob@company.com".casefold():
+       if ctx.content_item.email.casefold() == "bob@company.com".casefold():
-            ctx=ctx.MapTo(self._admin_username)
+            ctx = ctx.map_to(self._admin_username)
-            self._logger.debug('Mapped %s to %s', ctx.ContentItem.Email, ctx.MappedLocation.ToString())
+            self._logger.debug('Mapped %s to %s', ctx.content_item.email, str(ctx.mapped_location))
        return ctx
```

### Version 3 -> Version 4+ Registration Diff

```
-   plan_builder.mappings.add(IUser, SpecialUserMapping())
+   plan_builder.mappings.add(SpecialUserMapping())
```

## Transformers

There are a few changes that must be made to existing transformers for them to work in version 4+.

The imports have changed. All imports are now from the Python `tableau_migration` namespaces instead of the C# `Tableau.Migration` namespaces.

The `__namespace__` and `_dotnet_base` variables are no longer required.

The `Execute` function is now `transform` with the parameter being of the publishable type to transform.

For registration, the type is no longer required. Simply add the object to the transformer list.

### Version 3 -> Version 4+ Transformer Class Diff

```
-from Tableau.Migration.Interop.Hooks.Transformers import ISyncContentTransformer
-from Tableau.Migration.Content import IPublishableDataSource
+from typing import TypeVar
+from tableau_migration import (
+    ContentTransformerBase,
+    IPublishableWorkbook,
+    IPublishableDataSource)

-class EncryptExtractsDataSourceTransformer(ISyncContentTransformer[IPublishableDataSource]):
+class EncryptExtractsDataSourceTransformer(ContentTransformerBase[IPublishableDataSource]):
-    __namespace__ = "MyNamespace"

-    def Execute(self, ctx):
+    def transform(self, itemToTransform: IPublishableDataSource) -> IPublishableDataSource:
-       ctx.EncryptExtracts = True
+       itemToTransform.encrypt_extracts = True
-       return ctx
+       return itemToTransform
```

### Version 3 -> Version 4+ Registration Diff

```
-   plan_builder.transformers.add(IPublishableDataSource, EncryptExtractsDataSourceTransformer())
+   plan_builder.transformers.add(EncryptExtractsDataSourceTransformer())
```

---

# Logging

The Migration SDK has built-in support for logging. SDK consumers can configure logging levels[.NET](https://learn.microsoft.com/en-us/dotnet/api/microsoft.extensions.logging.loglevel), [Python](https://docs.python.org/3/howto/logging.html#logging-levels) and add more logging providers (called handlers in Python)[.NET](https://learn.microsoft.com/en-us/dotnet/core/extensions/logging-providers), [Python](https://docs.python.org/3/library/logging.handlers.html).

Internally, the SDK will log every successfully **Request**/**Response** as an **Information** message with the [**Http Request Method**](https://developer.mozilla.org/en-US/docs/Web/HTTP/Methods), the **Http Request Uri** and the [**Http Response Status Code**](https://developer.mozilla.org/en-US/docs/Web/HTTP/Status):

```
> Tableau.Migration.Net.NetworkTraceLogger: Information: HTTP GET "https://localhost/api/2.4/serverinfo" responded "OK".
```

It will also log every errored **Request**/**Response** as an **Error** message with the [**Http Request Method**](https://developer.mozilla.org/en-US/docs/Web/HTTP/Methods), the **Http Request Uri** and the **Error Message**:

```
> Tableau.Migration.Net.NetworkTraceLogger: Error: HTTP GET "https://localhost/api/2.4/serverinfo" failed. Error: "An error occurred while sending the request.".
```

As part of the included tracings, it is possible to configure the level of details for each log message by setting the following configuration parameters:

- Network.RequestsLoggingEnabled: Indicates whether the SDK logs request start events. The default value is enabled.
- Network.HeadersLoggingEnabled: Indicates whether the SDK logs request/response headers. The default value is disabled.
- Network.ContentLoggingEnabled: Indicates whether the SDK logs request/response content. The default value is disabled.
- Network.BinaryContentLoggingEnabled: Indicates whether the SDK logs request/response binary (not textual) content. The default value is disabled.
- Network.ExceptionsLoggingEnabled: Indicates whether the SDK logs network exceptions. The default value is disabled.

- Python Support
- C# Support

The Migration SDK supports logging with built-in providers like the one described in [Python Logging docs](https://docs.python.org/3/howto/logging.html).

### SDK default handler

The SDK adds a StreamHandler to the root logger by executing the following command:

```
logging.basicConfig(
    format = '%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    level = logging.INFO)
```

### Overriding default handler configuration

To override the default configuration, set the `force` parameter to `True`.

```
logging.basicConfig(
    force = True,
    format = '%(asctime)s|%(levelname)s|%(name)s -\t%(message)s',
    level = logging.WARNING)
```

##### Note

See [Logging Configuration](https://docs.python.org/3/library/logging.config.html) for advanced configuration guidance.

The Migration SDK supports logging with built-in or third-party providers such as the ones described in [.NET Logging Providers](https://learn.microsoft.com/en-us/dotnet/core/extensions/logging-providers). Refer to that article for guidance in your use case. Some basic examples are below.

### Adding your logging provider

You can add logging when you add the Migration SDK to the service collection.

#### Adding a default provider without configuration

```
services
    .AddTableauMigrationSdk()
    .AddLogging();
```

#### Adding a logging provider with configuration

This example adds NLog.

```
services
    .AddTableauMigrationSdk()
    .AddLogging(builder =>
    {
        builder.AddNLog();
    })
```

##### Note

See [LoggingServiceCollectionExtensions.AddLogging Method](https://learn.microsoft.com/en-us/dotnet/api/microsoft.extensions.dependencyinjection.loggingservicecollectionextensions.addlogging) for guidance on how to configure your logging provider.

---

# Plan Validation

## Plan Builder

The main input for a migration is through a migration plan. A plan builder is provided by the SDK to build a valid migration plan through a [fluent interface](https://en.wikipedia.org/wiki/Fluent_interface). The plan builder is able to find validation errors before the migration plan is built and executed.

### Validation

The migration engine does not enforce plan validation, but users are highly encouraged to validate migration plans and abort execution if validation errors are returned to prevent errors during migration. The **Validate** plan builder method is used to perform plan builder validation:

```
var validationResult = _planBuilder.Validate();
```

### Handling Validation Errors

If a validation error is detected, we recommend aborting the migration in an application appropriate manner. The following example checks for validation errors and logs them to the console:

```
if (!validationResult.Success)
    {
        _logger.LogError($"Migration plan validation failed.", validationResult.Errors);
        Console.WriteLine("Press any key to exit");
        Console.ReadKey();
        _appLifetime.StopApplication();
    }
```

Each validation error provides information on how to fix the error detected in the migration plan. Review the validation errors, adjust the plan builder as necessary, and re-run the migration.

---

# SDK Terminology

## Content Type

Content types are the various types of content that reside on Tableau Server or Tableau Cloud. For details about which content types the Migration SDK supports, see [Supported Content Types](https://help.tableau.com/current/api/migration_sdk/en-us/docs/supported_content_types.html). For unsupported content types, see [Data not supported by the Migration SDK](https://help.tableau.com/current/api/migration_sdk/en-us/docs/planning.html#data-not-supported-by-the-migration-sdk).

## Content Item

Item of a certain content type.

## Content Migration Action or Content Action

The action that migrates Content Items of a certain Content Type.

## Migration Plan

A migration plan describes how a migration should be done and what customizations must be done inflight. The `IMigrationPlan` interface defines the Migration Plan structure. See Basic Configuration for more guidance on the Migration Plan and how to use it.

## Plan Builder

This is the best way to build a migration plan. Calling the `Build()` method on the `IMigrationPlanBuilder` gives you a `MigrationPlan`.

## Manifest

The migration manifest describes the various Tableau data items found to migrate and their migration results. See `IMigrationManifest` for details.

## Manifest Serializer

The Migration SDK ships with a helpful serializer in both C# and Python. It serializes and deserializes migration manifests in JSON format.

## Migration Status

This is simply the status of the migration. See `MigrationCompletionStatus` for a list of statuses.

## Migration Result

This is the result generated after the migration has finished. It has two properties

1. `Manifest`
2. `Migration Status`

## Hook

A hook is a means of modifying the standard functionality of the Migration SDK. These modifications include filtering, mapping, transforming migration content and reacting to other SDK events.

---

# Troubleshooting

## Common issues

### Migration fails due to invalid credentials

1. Make sure credentials are correct in Migration SDK configuration.
2. If they are incorrect/absent, [Create a Personal Access Token (PAT)](https://help.tableau.com/current/server/en-us/security_personal_access_tokens.htm#:%7E:text=Create%20personal%20access%20tokens,-Users%20must%20create&text=Users%20with%20accounts%20on%20Tableau,have%20up%20to%2010%20PATs) and use them in the Migration SDK configuration.

### The migration has finished but I do not see the expected content on the destination

When the migration finishes you get a MigrationResult. The MigrationCompletionStatus should be `Canceled` or `FatalError`. In the Manifest, check

- Errors: The top level errors not related to any manifest entries.
- Entries: This is a collection of manifest entries that can be grouped by content type. Here are code snippets that log those errors. You can use this as general reference to process migration errors in your application.

C#

```
foreach (var type in ServerToCloudMigrationPipeline.ContentTypes)
{
    var contentType = type.ContentType;

    _logger.LogInformation($"## {contentType.Name} ##");

    // Manifest entries can be grouped based on content type.
    foreach (var entry in result.Manifest.Entries.ForContentType(contentType))
    {
        _logger.LogInformation($"{contentType.Name} {entry.Source.Location} Migration Status: {entry.Status}");

        if (entry.Errors.Any())
        {
            _logger.LogError($"## {contentType.Name} Errors detected! ##");
            foreach (var error in entry.Errors)
            {
                _logger.LogError(error, "Processing Error.");
            }
        }

        if (entry.Destination is not null)
        {
            _logger.LogInformation($"{contentType.Name} {entry.Source.Location} migrated to {entry.Destination.Location}");
        }
    }
}
```

Python

```
for type in ServerToCloudMigrationPipeline.ContentTypes:
    content_type = type.ContentType
    _logger.LogInformation(f"## {content_type.Name} ##")
    for entry in result.manifest.entries.ForContentType(content_type):
        _logger.LogInformation(f"{content_type.Name} {entry.Source.Location} Migration Status: {entry.Status}")
        if entry.Errors:
            _logger.LogError(f"## {content_type.Name} Errors detected! ##")
            for error in entry.Errors:
                _logger.LogError(error, "Processing Error.")
        if entry.Destination is not None:
            _logger.LogInformation(f"{content_type.Name} {entry.Source.Location} migrated to {entry.Destination.Location}")
```

### Python - The SDK isn't loading the *.env* configuration

Environment variables must be set in the system the Python application runs in. This can be done through the OS itself, or by 3rd party libraries. The SDK will load the environment configuration on its **__init__** process.

For the case of the library [dotenv](https://pypi.org/project/python-dotenv/), it is required to execute the command **load_dotenv()** before referring to any **tableau_migration** code.

```
# Used to load environment variables
from dotenv import load_dotenv

# Load the environment variables before importing tableau_migration
load_dotenv()

# first tableau_migration reference
import tableau_migration

# The SDK will not recognize the .env file values
# Don't load the values here
# load_dotenv()
```

### Default Permissions for Virtual Connections and Flows

When migrating from Tableau Server to Tableau Cloud without Data Management Add-on, applying default permissions for `All Users` for Virtual Connections and Flows may not be desired. To skip applying default permissions to these specific content types, the migration must be configured using `DefaultPermissionsContentTypeOptions`. See DefaultPermissionsContentTypeOptions for further details on configuration.

## Errors and Warnings

This section provides a list of potential error and warning log messages that you may encounter in the logs. Each entry includes a description to assist you in debugging.

### Warning: `Could not add a user to the destination Group [group name]. Reason: Could not find the destination user for [user name].`

This warning message indicates that the `GroupUsersTransformer` was unable to add the user, denoted as `[user name]`, to the group, denoted as `[group name]`.

This situation can occur if a user was excluded by a custom filter, but was not mapped to another user. If a custom filter was implemented based on the `ContentFilterBase<IUser>` class, then debug logging is already available.

To resolve this issue, enable debug logging to identify which filter is excluding the user. Then, add a mapping to an existing user using the `ContentMappingBase<IUser>` class.

### Error (manifest) migrating `Guest` users

`Guest` users are not supported on Tableau Cloud. They are only on Servers with the legacy Core based licensing. So, the Migration SDK cannot migrate them. To mitigate the problem, you can do one of these things

1. If the users have no associated permissions on content items, you can write a User filter based on their SiteRole (PySiteRoles/SiteRoles).
2. If the users do have associate permissions on content items, you can write a mapping for each of them to a different user.

### Warning: `Embedded Managed OAuth Credentials migration is not supported. They will be converted to saved credentials for[workbook/data source] [name] at [location]. The connection IDs are [list of connection IDs].`

This warning message indicates that the Migration SDK did not migrate a workbook/data source's [Managed OAuth Embedded Credentials](https://help.tableau.com/current/server/en-us/protected_auth.htm#defaultmanaged-keychain-connectors). They will be automatically converted to saved credentials at the destination. Users will need to re-enter credentials the first time they use the workbook/ data source. All other types of embedded credentials are migrated as they are.

### Error `Content migration data could not be found for site '[Site ID]'.`

This error message indicates that you need to authorize credential migration before migrating content with embedded credentials. See the [Pre-Migration Checklist](https://help.tableau.com/current/api/migration_sdk/en-us/docs/how_to_migrate.html) for more details.

---

# User and Group authentication

[Tableau Server](https://help.tableau.com/current/server/en-us/security_auth.htm) and [Tableau Cloud](https://help.tableau.com/current/online/en-us/security_auth.htm) support different authentication types. The Migration SDK supports authentication types listed in AuthenticationTypes out of the box.

## Defaults

The default authentication type for users is `ServerDefault`. It is set by the automatically registered `UserAuthenticationTypeTransformer`.

## Server to Cloud

It is possible to set the authentication type on users and groups. The `ServerToCloudMigrationPlanBuilder` contains methods to support mapping Server users and groups to Cloud authentication types.

### SAML and Tableau ID specific

1. `WithSamlAuthenticationType(string domain, string? idpConfigurationName = null)` : Adds mappings for user and group domains based on the SAML authentication type with a domain supplied. When a site has [multiple SAML authentication types](https://help.tableau.com/current/online/en-us/security_auth.htm#multiple_idp) enabled, the IdP configuration name should be supplied.
2. `WithTableauIdAuthenticationType(bool mfa = true, string? idpConfigurationName = null)`: Adds mappings for user and group domains based on the Tableau ID authentication type with or without multi-factor authentication.

### Tableau Cloud Usernames

The `WithTableauCloudUsernames()` method and its overloads allow you to supply an email domain or your own implementation of `ITableauCloudUsernameMapping` for Tableau Cloud user names.

### General methods

The `WithAuthenticationType()` method and its overloads allow you to supply your chosen authentication type with your implementation of `IAuthenticationTypeDomainMapping`. When a site has [multiple authentication types](https://help.tableau.com/current/online/en-us/security_auth.htm#multiple_idp) enabled the IdP configuration name should be used as the authentication type. Otherwise a `AuthenticationTypes` value should be used.

## Custom Mapping

You can also build your own mapping to supply to the appropriate `WithAuthenticationType()` overload. See Sample EmailDomainMapping for example code.

---

## Samples (Runable Example Code)

---

# Sample: Batch Migration Logging

This example demonstrates how to log migration batch item statuses using a batch migration completed hook.

- Python
- C#

#### Batch Migration Completed Hook Class

```
import logging
from typing import TypeVar
from tableau_migration import(
    ContentBatchMigrationCompletedHookBase,
    IContentBatchMigrationResult,
    IUser
    )

T = TypeVar("T")

class LogMigrationBatchesHook(ContentBatchMigrationCompletedHookBase[T]):
    def __init__(self) -> None:
        super().__init__()
        self._logger = logging.getLogger(__name__)

    def execute(self, ctx: IContentBatchMigrationResult[T]) -> IContentBatchMigrationResult[T]:

        item_status = ""
        for item in ctx.item_results:
            item_status += "%s: %s".format(item.manifest_entry.source.location, item.manifest_entry.status)

        self._logger.info("%s batch of %d item(s) completed:\n%s", ctx._content_type, ctx.item_results.count, item_status)

        pass

class LogMigrationBatchesHookForUsers(ContentBatchMigrationCompletedHookBase[IUser]):
    def __init__(self) -> None:
        super().__init__()
        self._content_type = "User";

class LogMigrationBatchesHookForGoups(ContentBatchMigrationCompletedHookBase[IUser]):
    def __init__(self) -> None:
        super().__init__()
        self._content_type = "Group";
```

#### Registration

Learn more.

```
plan_builder.hooks.add(LogMigrationBatchesHookForUsers)
plan_builder.hooks.add(LogMigrationBatchesHookForGroups)
```

#### Batch Migration Completed Hook Class

```
public class LogMigrationBatchesHook<T> : IContentBatchMigrationCompletedHook<T>
    where T : IContentReference
{
    private readonly ILogger<LogMigrationBatchesHook<T>> _logger;

    public LogMigrationBatchesHook(ILogger<LogMigrationBatchesHook<T>> logger)
    {
        _logger = logger;
    }

    public Task<IContentBatchMigrationResult<T>?> ExecuteAsync(IContentBatchMigrationResult<T> ctx, CancellationToken cancel)
    {
        _logger.LogInformation(
            "{ContentType} batch of {Count} item(s) completed:{NewLine}{Statuses}",
            typeof(T).Name,
            ctx.ItemResults.Count,
            Environment.NewLine,
            String.Join(Environment.NewLine, ctx.ItemResults.Select(r => $"{r.ManifestEntry.Source.Location}: {r.ManifestEntry.Status}")));

        return Task.FromResult<IContentBatchMigrationResult<T>?>(ctx);
    }
}
```

#### Registration

Learn more.

```
_planBuilder.Hooks.Add<LogMigrationBatchesHook<IUser>>();
_planBuilder.Hooks.Add<LogMigrationBatchesHook<IProject>>();
_planBuilder.Hooks.Add<LogMigrationBatchesHook<IDataSource>>();
_planBuilder.Hooks.Add<LogMigrationBatchesHook<IWorkbook>>();
_planBuilder.Hooks.Add<LogMigrationBatchesHook<ICloudExtractRefreshTask>>();
```

#### Dependency Injection

Learn more.

```
services.AddScoped(typeof(LogMigrationBatchesHook<>));
```

---

# Sample: Bulk logging

In the following example, a bulk post-publish hook logs published items.

- Python
- C#

#### Bulk Post-Publish Hook Class

```
import logging
from tableau_migration import (
    BulkPostPublishHookBase,
    BulkPostPublishContext,
    IDataSource)

class BulkLoggingHookForDataSources(BulkPostPublishHookBase[IDataSource]):
    def __init__(self) -> None:
        super().__init__()

        # Create a logger for this class
        self._logger = logging.getLogger(__name__)

    def execute(self, ctx: BulkPostPublishContext[IDataSource]) -> BulkPostPublishContext[IDataSource]:
        # Log the number of items published in the batch.
        self._logger.info("Published %d IDataSource item(s).", ctx.published_items.count)
        return None
```

#### Registration

Learn more.

```
plan_builder.hooks.add(BulkLoggingHookForDataSources)
```

#### Bulk Post-Publish Hook Class

```
public class BulkLoggingHook<T> : BulkPostPublishHookBase<T>
{
    private readonly ILogger<BulkLoggingHook<T>> _logger;

    public BulkLoggingHook(ILogger<BulkLoggingHook<T>> logger)
    {
        _logger = logger;
    }

    public override Task<BulkPostPublishContext<T>?> ExecuteAsync(BulkPostPublishContext<T> ctx, CancellationToken cancel)
    {
        // Log the number of items published in the batch.
        _logger.LogInformation(
            "Published {Count} {ContentType} item(s).",
            ctx.PublishedItems.Count,
            typeof(T).Name);

        return Task.FromResult<BulkPostPublishContext<T>?>(ctx);
    }
}
```

#### Registration

Learn more.

```
_planBuilder.Hooks.Add<BulkLoggingHook<IUser>>();
```

#### Dependency Injection

Learn more.

```
services.AddScoped(typeof(BulkLoggingHook<>));
```

---

# Sample: Filter Custom Views by Shared Status

In this example, the custom views currently shared to all users are excluded from migration.

- Python
- C#

#### Filter Class

```
from tableau_migration import (
    ICustomView,
    ContentFilterBase,
    ContentFilterContextItem,
    FilterStatus)

class SharedCustomViewFilter(ContentFilterBase[ICustomView]):
    def filter(self, item: ContentFilterContextItem[ICustomView]) -> None:
        if item.source_item.shared == True:
            item.status = FilterStatus.SKIP
```

#### Registration

Learn more.

```
plan_builder.filters.add(SharedCustomViewFilter)
```

#### Filter Class

```
public class SharedCustomViewFilter : ContentFilterBase<ICustomView>
{
    public SharedCustomViewFilter(
        ISharedResourcesLocalizer localizer,
        ILogger<IContentFilter<ICustomView>> logger)
            : base(localizer, logger) { }

    public override void Filter(ContentFilterContextItem<ICustomView> item)
    {
        if (item.SourceItem.Shared)
        {
            item.Status = FilterStatus.Skip;
        }
    }
}
```

#### Registration

Learn more.

```
_planBuilder.Filters.Add<SharedCustomViewFilter, ICustomView>();
```

#### Dependency Injection

Learn more.

```
services.AddScoped<SharedCustomViewFilter>();
```

---

# Sample: Filter Projects by Name

In this example, the project named `Default` is filtered out.

- Python
- C#

#### Filter Class

```
from tableau_migration import (
    ContentFilterBase,
    ContentFilterContextItem,
    FilterStatus,
    IProject)

class DefaultProjectFilter(ContentFilterBase[IProject]):

    def filter(self, item: ContentFilterContextItem[IProject]) -> None:
        if item.source_item.name.casefold() == 'Default'.casefold():
            item.status = FilterStatus.CASCADE_SKIP
```

#### Registration

```
plan_builder.filters.add(DefaultProjectFilter)
```

See hook registration for more details.

#### Filter Class

```
public class DefaultProjectsFilter : ContentFilterBase<IProject>
{
    public DefaultProjectsFilter(
        ISharedResourcesLocalizer localizer,
        ILogger<IContentFilter<IProject>> logger) : base(localizer, logger) { }

    public override void Filter(ContentFilterContextItem<IProject> item)
    {
        if (string.Equals(item.SourceItem.Name, "default", System.StringComparison.OrdinalIgnoreCase))
        {
            item.Status = FilterStatus.CascadeSkip;
        }
    }
}
```

#### Registration

Learn more.

```
_planBuilder.Filters.Add<DefaultProjectsFilter, IProject>();
```

#### Dependency Injection

Learn more.

```
services.AddScoped<DefaultProjectsFilter>();
```

---

# Sample: Filter Users by SiteRole

In this example, all unlicensed users are excluded from migration.

- Python
- C#

#### Filter Class

```
from tableau_migration import (
    ContentFilterBase,
    ContentFilterContextItem,
    FilterStatus,
    IUser,
    SiteRoles)

class UnlicensedUserFilter(ContentFilterBase[IUser]):

    def filter(self, item: ContentFilterContextItem[IUser]) -> None:
        if item.source_item.license_level.casefold() == SiteRoles.UNLICENSED.casefold():
            item.status = FilterStatus.SKIP
```

#### Registration

```
plan_builder.filters.add(UnlicensedUserFilter)
```

See hook registration for more details.

#### Filter Class

```
public class UnlicensedUsersFilter : ContentFilterBase<IUser>
{
    public UnlicensedUsersFilter(
        ISharedResourcesLocalizer localizer,
        ILogger<IContentFilter<IUser>> logger)
            : base(localizer, logger) { }

    public override void Filter(ContentFilterContextItem<IUser> item)
    {
        if (string.Equals(item.SourceItem.SiteRole, SiteRoles.Unlicensed, StringComparison.OrdinalIgnoreCase))
        {
            item.Status = FilterStatus.Skip;
        }
    }
}
```

#### Registration

Learn more.

```
_planBuilder.Filters.Add<UnlicensedUsersFilter, IUser>();
```

#### Dependency Injection

Learn more.

```
services.AddScoped<UnlicensedUsersFilter>();
```

---

# Sample: Change Projects

In this example, source data sources and workbooks in a project named `Test` are migrated to the destination's `Production` project.

Both the C# and Python mapping classes inherit from a base class that handles most of the work, and then create an `IWorkbook` and `IDataSource` version.

- Python
- C#

#### Mapping Class

```
from typing import TypeVar
from tableau_migration import(
    IWorkbook,
    IDataSource,
    ContentMappingContext,
    ContentMappingBase)

T = TypeVar("T")

class ChangeProjectMapping(ContentMappingBase[T]):

    def map(self, ctx: ContentMappingContext[T]) -> ContentMappingContext[T]:
        # Get the container (project) location for the content item.
        container_location = ctx.content_item.location.parent()

        # We only want to map content items whose project name is "Test".
        if not container_location.name.casefold() == "Test".casefold():
            return ctx

        # Build the new project location.
        new_container_location = container_location.rename("Production")

        # Build the new content item location.
        new_location = new_container_location.append(ctx.content_item.name)

        # Map the new content item location.
        ctx = ctx.map_to(new_location)

        return ctx

# Create the workbook version of the templated ChangeProjectMapping class
class ChangeProjectMappingForWorkbooks(ChangeProjectMapping[IWorkbook]):
    pass

# Create the datasource version of the templated ChangeProjectMapping class
class ChangeProjectMappingForDataSources(ChangeProjectMapping[IDataSource]):
    pass
```

#### Registration

```
plan_builder.mappings.add(ChangeProjectMappingForWorkbooks)
plan_builder.mappings.add(ChangeProjectMappingForDataSources)
```

See hook registration for more details.

#### Mapping Class

```
public class ChangeProjectMapping<T> : ContentMappingBase<T>
    where T : IContentReference, IMappableContainerContent
{
    private static readonly StringComparer StringComparer = StringComparer.OrdinalIgnoreCase;

    private readonly ILogger<IContentMapping<T>>? _logger;

    public ChangeProjectMapping(ISharedResourcesLocalizer? localizer, ILogger<IContentMapping<T>>? logger) : base(localizer, logger)
    {
        _logger = logger;
    }

    public override Task<ContentMappingContext<T>?> MapAsync(ContentMappingContext<T> ctx, CancellationToken cancel)
    {
        // Get the container (project) location for the content item.
        var containerLocation = ctx.ContentItem.Location.Parent();

        // We only want to map content items whose project name is "Test".
        if (!StringComparer.Equals("Test", containerLocation.Name))
        {
            return ctx.ToTask();
        }

        // Build the new project location.
        var newContainerLocation = containerLocation.Rename("Production");

        // Build the new content item location.
        var newLocation = newContainerLocation.Append(ctx.ContentItem.Location.Name);

        // Map the new content item location.
        ctx = ctx.MapTo(newLocation);

        _logger?.LogInformation(
            "{ContentType} mapped from {OldLocation} to {NewLocation}.",
            typeof(T).Name,
            ctx.ContentItem.Location,
            ctx.MappedLocation);

        return ctx.ToTask();
    }

    public async Task<ContentMappingContext<IDataSource>?> MapAsync(ContentMappingContext<IDataSource> ctx, CancellationToken cancel)
        => await MapAsync(ctx, cancel);

    public async Task<ContentMappingContext<IWorkbook>?> MapAsync(ContentMappingContext<IWorkbook> ctx, CancellationToken cancel)
        => await MapAsync(ctx, cancel);
}
```

#### Registration

Learn more.

```
_planBuilder.Mappings.Add<ChangeProjectMapping<IDataSource>, IDataSource>();
_planBuilder.Mappings.Add<ChangeProjectMapping<IWorkbook>, IWorkbook>();
```

#### Dependency Injection

Learn more.

```
services.AddScoped<ChangeProjectMapping<IWorkbook>>();
services.AddScoped<ChangeProjectMapping<IDataSource>>();
```

---

# Sample: Rename Projects

In this example, the source project named `Test` is renamed to `Production` on the destination.

- Python
- C#

#### Mapping Class

```
from tableau_migration import(
    IProject,
    ContentMappingBase,
    ContentMappingContext)

class ProjectRenameMapping(ContentMappingBase[IProject]):
    def map(self, ctx: ContentMappingContext[IProject]) -> ContentMappingContext[IProject]:
        if not ctx.content_item.name.casefold() == "Test".casefold():
            return ctx

        new_location = ctx.content_item.location.rename("Production")

        ctx = ctx.map_to(new_location)

        return ctx
```

#### Registration

```
plan_builder.mappings.add(ProjectRenameMapping)
```

See hook registration for more details.

#### Mapping Class

```
public class ProjectRenameMapping : ContentMappingBase<IProject>
{
    public ProjectRenameMapping(ISharedResourcesLocalizer localizer, ILogger<IContentMapping<IProject>> logger) : base(localizer, logger)
    { }

    public override async Task<ContentMappingContext<IProject>?> MapAsync(ContentMappingContext<IProject> ctx, CancellationToken cancel)
    {
        if (!String.Equals("Test", ctx.ContentItem.Name, StringComparison.OrdinalIgnoreCase))
        {
            return ctx;
        }

        var newLocation = ctx.ContentItem.Location.Rename("Production");

        ctx = ctx.MapTo(newLocation);

        return await ctx.ToTask();
    }
}
```

#### Registration

Learn more.

```
_planBuilder.Mappings.Add<ProjectRenameMapping, IProject>();
```

#### Dependency Injection

Learn more.

```
services.AddScoped<ProjectRenameMapping>();
```

---

# Sample EmailDomainMapping

This example is for a hypothetical scenario where the Tableau Server usernames are the same as the user part of the email. It uses a configuration class to supply the email domain.

- Python
- C#

#### Mapping Class

```
from tableau_migration import(
    IUser,
    TableauCloudUsernameMappingBase,
    ContentMappingContext)

class EmailDomainMapping(TableauCloudUsernameMappingBase):
    def map(self, ctx: ContentMappingContext[IUser]) -> ContentMappingContext[IUser]:
        _email_domain: str = "@mycompany.com"

        _tableau_user_domain = ctx.mapped_location.parent()

        # Re-use an existing email if it already exists.
        if ctx.content_item.email:
            return ctx.map_to(_tableau_user_domain.append(ctx.content_item.email))

        # Takes the existing username and appends the domain to build the email
        new_email = ctx.content_item.name + _email_domain
        return ctx.map_to(_tableau_user_domain.append(new_email))
```

#### Registration

Learn more.

See the line with `with_tableau_cloud_usernames`.

```
plan_builder = plan_builder \
                .from_source_tableau_server(
                    server_url=config['SOURCE']['URL'],
                    site_content_url=config['SOURCE']['SITE_CONTENT_URL'],
                    access_token_name=config['SOURCE']['ACCESS_TOKEN_NAME'],
                    access_token=os.environ.get('TABLEAU_MIGRATION_SOURCE_TOKEN', config['SOURCE']['ACCESS_TOKEN'])) \
                .to_destination_tableau_cloud(
                    pod_url=config['DESTINATION']['URL'],
                    site_content_url=config['DESTINATION']['SITE_CONTENT_URL'],
                    access_token_name=config['DESTINATION']['ACCESS_TOKEN_NAME'],
                    access_token=os.environ.get('TABLEAU_MIGRATION_DESTINATION_TOKEN', config['DESTINATION']['ACCESS_TOKEN'])) \
                .for_server_to_cloud() \
                .with_tableau_id_authentication_type() \
                # You can add authentication type mappings here
                .with_tableau_cloud_usernames(EmailDomainMapping)
```

This uses a configuration class to supply the email domain.

#### Mapping Class

```
namespace Csharp.ExampleApplication.Hooks.Mappings
{
    /// <summary>
    /// Mapping that appends an email domain to a username.
    /// </summary>
    public class EmailDomainMapping :
        ContentMappingBase<IUser>, // Base class to build mappings for content types
        ITableauCloudUsernameMapping
    {

        private readonly string _domain;

        /// <summary>
        /// Creates a new <see cref="EmailDomainMapping"/> object.
        /// </summary>
        /// <param name="optionsProvider">The options for this Mapping.</param>
        public EmailDomainMapping(
            IMigrationPlanOptionsProvider<EmailDomainMappingOptions> optionsProvider,
            ISharedResourcesLocalizer localizer,
            ILogger<EmailDomainMapping> logger)
                : base(localizer, logger)
        {
            _domain = optionsProvider.Get().EmailDomain;
        }

        /// <summary>
        /// Adds an email to the user if it doesn't exist.
        /// This is where the main logic of the mapping should reside.
        /// </summary>
        public override Task<ContentMappingContext<IUser>?> MapAsync(ContentMappingContext<IUser> userMappingContext, CancellationToken cancel)
        {
            var domain = userMappingContext.MappedLocation.Parent();

            // Re-use an existing email if it already exists.
            if (!string.IsNullOrEmpty(userMappingContext.ContentItem.Email))
                return userMappingContext.MapTo(domain.Append(userMappingContext.ContentItem.Email)).ToTask();

            // Takes the existing username and appends the domain to build the email
            var testEmail = $"{userMappingContext.ContentItem.Name}@{_domain}";
            return userMappingContext.MapTo(domain.Append(testEmail)).ToTask();
        }
    }
}
```

#### Configuration Class

```
namespace Csharp.ExampleApplication.Hooks.Mappings
{
    public sealed class EmailDomainMappingOptions
    {
        public string EmailDomain { get; set; } = string.Empty;
    }
}
```

#### Registration

Learn more.

See the line with `WithTableauCloudUsernames`.

```
// Use the methods on your plan builder to add configuration and make customizations.
_planBuilder = _planBuilder
    .FromSourceTableauServer(_options.Source.ServerUrl, _options.Source.SiteContentUrl, _options.Source.AccessTokenName, Environment.GetEnvironmentVariable("TABLEAU_MIGRATION_SOURCE_TOKEN") ?? string.Empty)
    .ToDestinationTableauCloud(_options.Destination.ServerUrl, _options.Destination.SiteContentUrl, _options.Destination.AccessTokenName, Environment.GetEnvironmentVariable("TABLEAU_MIGRATION_DESTINATION_TOKEN") ?? string.Empty)
    .ForServerToCloud()
    .WithTableauIdAuthenticationType()
    // You can add authentication type mappings here
    .WithTableauCloudUsernames<EmailDomainMapping>();
```

#### Dependency Injection

Learn more.

```
services.AddScoped<EmailDomainMapping>();
```

---

# Sample: Migration Action Logging

This sample illustrates how to implement migration action logging, capturing the statuses of migration actions upon completion.

- Python
- C#

### Migration Action Completed Hook Class

To log migration action statuses in Python, you can utilize the following hook class:

```
import logging
from tableau_migration import(
    MigrationActionCompletedHookBase,
    IMigrationActionResult
    )

class LogMigrationActionsHook(MigrationActionCompletedHookBase):
    def __init__(self) -> None:
        super().__init__()

        # Create a logger for this class
        self._logger = logging.getLogger(__name__)

    def execute(self, ctx: IMigrationActionResult) -> IMigrationActionResult:
        if(ctx.success):
            self._logger.info("Migration action completed successfully.")
        else:
            all_errors = "\n".join(ctx.errors)
            self._logger.warning("Migration action completed with errors:\n%s", all_errors)

        return None
```

### Registration

```
plan_builder.hooks.add(LogMigrationActionsHookForUsers)
plan_builder.hooks.add(LogMigrationActionsHookForGroups)
```

See hook registration for more details.

### Migration Action Completed Hook Class

In C#, you can implement the migration action completed hook as demonstrated below:

```
public class LogMigrationActionsHook : IMigrationActionCompletedHook
{
    private readonly ILogger<LogMigrationActionsHook> _logger;

    public LogMigrationActionsHook(ILogger<LogMigrationActionsHook> logger)
    {
        _logger = logger;
    }

    public Task<IMigrationActionResult?> ExecuteAsync(IMigrationActionResult ctx, CancellationToken cancel)
    {
        if (ctx.Success)
        {
            _logger.LogInformation("Migration action completed successfully.");
        }
        else
        {
            _logger.LogWarning(
                "Migration action completed with errors:{NewLine}{Errors}",
                Environment.NewLine,
                String.Join(Environment.NewLine, ctx.Errors.Select(e => e.ToString())));
        }

        return Task.FromResult<IMigrationActionResult?>(ctx);
    }
}
```

### Registration

To register the hook in C#, follow the instructions provided in the documentation.

```
_planBuilder.Hooks.Add<LogMigrationActionsHook>();
```

### Dependency Injection

Learn more about dependency injection here.

```
services.AddScoped<LogMigrationActionsHook>();
```

---

# Sample: Update permissions

In the following example, write permissions for content with a `Production` tag will be set to `Deny` as a separate post-publish step. To modify permissions that are automatically updated as part of the standard post-publish step, see the modify permissions transformer sample.

- Post-Publish Hook Class
- Registration
- Dependency Injection

```
public class UpdatePermissionsHook<TPublish, TResult> : PermissionPostPublishHookBase<TPublish, TResult>
    where TResult : IPermissionsContent, IWithTags
{
    private static readonly StringComparer StringComparer = StringComparer.OrdinalIgnoreCase;

    private readonly ILogger<UpdatePermissionsHook<TPublish, TResult>> _logger;

    public UpdatePermissionsHook(IMigration migration, IContentTransformerRunner transformerRunner,
        ILogger<UpdatePermissionsHook<TPublish, TResult>> logger)
        : base(migration, transformerRunner)
    {
        _logger = logger;
    }

    /// <summary>
    /// Gets whether the content item's permissions should be updated.
    /// The logic here can be customized for other scenarios.
    /// </summary>
    /// <param name="item">The content item to evaluate</param>
    /// <returns>True to update the item's permissions, false otherwise.</returns>
    private static bool ShouldUpdatePermissions(TResult item)
    {
        // Only update permissions for content with a "Production" tag.
        var hasProductionTag = item.Tags.Any(t => StringComparer.Equals("Production", t.Label));

        return hasProductionTag;
    }

    /// <summary>
    /// Updates the capability (permission).
    /// The logic here can be customized for other scenarios.
    /// </summary>
    /// <param name="contentItem">The content item.</param>
    /// <param name="capabilities">The capability collection to update.</param>
    private bool UpdateCapabilities(TResult contentItem, HashSet<ICapability> capabilities)
    {
        var capabilityToUpdate = PermissionsCapabilityNames.Write;

        var removedCount = capabilities.RemoveWhere(c =>
            StringComparer.Equals(capabilityToUpdate, c.Name));

        if (removedCount == 0)
            return false;

        // Add the write/deny permission
        var updatedCapability = new Capability(capabilityToUpdate, PermissionsCapabilityModes.Deny);

        capabilities.Add(updatedCapability);

        _logger.LogInformation(
            "Set {ContentType} {ContentItem}'s {PermissionName} permission to {PermissionMode}.",
            typeof(TResult).Name,
            contentItem.Location,
            capabilityToUpdate,
            updatedCapability.Mode);

        return true;
    }

    public override async Task<ContentItemPostPublishContext<TPublish, TResult>?> ExecuteAsync(ContentItemPostPublishContext<TPublish, TResult> ctx, CancellationToken cancel)
    {
        // If parent project permissions are locked we can't update them.
        if (await ParentProjectLockedAsync(ctx, cancel))
        {
            return ctx;
        }

        // Since we're updating content after publish, our changes will be made to the destination item.
        var contentItem = ctx.DestinationItem;

        if (!ShouldUpdatePermissions(contentItem))
        {
            return ctx;
        }

        // Get the content item's current permissions.
        var permissionsResult = await Migration.Destination.GetPermissionsAsync<TResult>(contentItem, cancel);

        if (!permissionsResult.Success)
        {
            ctx.ManifestEntry.SetFailed(permissionsResult.Errors);
            return ctx;
        }

        var permissions = permissionsResult.Value;

        var hasUpdates = false;

        // Loop through the permission items to find/update the capabilities.
        foreach (var granteeCapability in permissions.GranteeCapabilities)
        {
            if (UpdateCapabilities(ctx.DestinationItem, granteeCapability.Capabilities))
                hasUpdates = true;
        }

        // If we haven't made any changes we can skip this part.
        if (hasUpdates)
        {
            // Update the content's permissions with our updated ones.
            var updatePermissionsResult = await Migration.Destination.UpdatePermissionsAsync<TResult>(
                contentItem,
                permissions,
                cancel);

            if (!updatePermissionsResult.Success)
            {
                ctx.ManifestEntry.SetFailed(updatePermissionsResult.Errors);
            }
        }

        return ctx;
    }
}
```

```
_planBuilder.Hooks.Add<UpdatePermissionsHook<IPublishableDataSource, IDataSourceDetails>>();
_planBuilder.Hooks.Add<UpdatePermissionsHook<IPublishableWorkbook, IWorkbookDetails>>();
```

```
services.AddScoped(typeof(UpdatePermissionsHook<,>));
```

---

# Sample: Filter Data Source Connections

This sample illustrates how to filter published data sources that use a given connection type. It uses a pulled hook as the connection details are not available until the data source has been fully retrieved.

- Python
- C#

#### Filter Class

```
from typing import Optional

from tableau_migration import (
    ContentItemPulledContext,
    ContentItemPulledHookBase,
    FilterStatus,
    IPublishableDataSource)

class DataSourceConnectionPulled(ContentItemPulledHookBase[IPublishableDataSource]):

    def execute(self, ctx: ContentItemPulledContext[IPublishableDataSource]) -> Optional[ContentItemPulledContext[IPublishableDataSource]]:
        if any(c.type.casefold() == "postgres".casefold() for c in ctx.pulled_item.connections):
            ctx.status = FilterStatus.CASCADE_SKIP

        return ctx
```

#### Registration

```
plan_builder.hooks.add(DataSourceConnectionPulled)
```

See hook registration for more details.

#### Filter Class

```
public class DataSourceConnectionPulled : ContentItemPulledHookBase<IPublishableDataSource>
{
    public override Task<ContentItemPulledContext<IPublishableDataSource>?> ExecuteAsync(ContentItemPulledContext<IPublishableDataSource> ctx, CancellationToken cancel)
    {
        if (ctx.PulledItem.Connections.Any(c => string.Equals(c.Type, "postgres", StringComparison.OrdinalIgnoreCase)))
        {
            ctx.Status = FilterStatus.CascadeSkip;
        }

        return ctx.ToTask();
    }
}
```

#### Registration

Learn more.

#### Dependency Injection

Learn more.

```
services.AddScoped<DataSourceConnectionPulled>();
```

---

# Sample: Action URL XML Transformer

This sample demonstrates how to read and write XML to update the workbook files. XML transformers should use the `XmlContentTransformerBase` base class that handles parsing the file XML and saving the modified XML back to the file to be published.

XML transformers require additional resource overhead to execute, so care should be taken when developing them.

- Due to encryption, XML transformers require the file to be loaded into memory to modify. For large files, such as those with large extracts, this can require significant memory.
- Python XML transformers each require extra processing as the parsed XML is converted from a .NET representation to a Python representation. For files with large XML content this can require significant time and memory. Combining multiple Python XML transformers can minimize this impact.

In general, resource overhead can be mimized by implementing the `NeedsXmlTransforming`/`needs_xml_transforming` method, which allows the transformer to use the metadata of the content item to determine whether the XML file needs to be loaded.

XML transformers are provided with "raw" XML and no file format validation is performed by the SDK. Care should be taken when modifying workbook or other files that the changes do not result in content that is valid XML but invalid by the file format. File format errors can lead to migration errors during publishing, and can also cause errors that are only apparent after the migration is complete and reported success.

- Python
- C#

### Transformer Class

To update action URLs in Python, you can use the following transformer class:

```
from xml.etree import ElementTree
from tableau_migration import (
    IPublishableWorkbook,
    XmlContentTransformerBase
)

class ActionUrlXmlTransformer(XmlContentTransformerBase[IPublishableWorkbook]):

    def needs_xml_transforming(self, ctx: IPublishableWorkbook) -> bool:
        # Returning false prevents the transform method from running.
        # Implementing this method potentially allows workbooks to migrate
        # without loading the file into memory, improving migration speed.
        return True

    def transform(self, ctx: IPublishableWorkbook, xml: ElementTree.Element) -> None:
        # Changes to the XML are saved back to the workbook file before publishing.
        for action_link in xml.findall("actions/*/link"):
            action_link.set("expression", action_link.get("expression").replace("127.0.0.1", "testserver"))
```

### Registration

```
plan_builder.transformers.add(ActionUrlXmlTransformer)
```

See hook registration for more details.

### Transformer Class

In C#, the transformer class for adjusting action URLs is implemented as follows:

```
public class ActionUrlXmlTransformer : XmlContentTransformerBase<IPublishableWorkbook>
{
    protected override bool NeedsXmlTransforming(IPublishableWorkbook ctx)
    {
        /*
         * Returning false prevents TransformAsync from running.
         * Implementing this method potentially allows workbooks to migrate without
         * loading the file into memory, improving migration speed.
         */
        return true;
    }

    public override Task TransformAsync(IPublishableWorkbook ctx, XDocument xml, CancellationToken cancel)
    {
        // Changes to the XML are saved back to the workbook file before publishing.
        foreach (var actionLink in xml.XPathSelectElements("//actions/*/link"))
        {
            actionLink.SetAttributeValue("expression", actionLink.Attribute("expression")?.Value?.Replace("127.0.0.1", "testserver"));
        }

        return Task.CompletedTask;
    }
}
```

### Registration

To register the transformer in C#, follow the guidance provided in the documentation.

```
_planBuilder.Transformers.Add<ActionUrlXmlTransformer, IPublishableWorkbook>();
```

### Dependency Injection

Learn more about dependency injection here.

```
services.AddScoped<ActionUrlXmlTransformer>();
```

---

# Sample: Changing Default Users for Custom View

This sample illustrates how to change the default users for a custom view.

Both the Python and C# transformer classes inherit from a base class that handles the core functionality, then create versions for `IPublishableCustomView`.

- Python
- C#

### Transformer Class

To implement the tag addition in Python, you can utilize the following transformer class:

```
from tableau_migration import (
    ContentTransformerBase,
    IContentReference,
    IPublishableCustomView
)

class CustomViewDefaultUsersTransformer(ContentTransformerBase[IPublishableCustomView]):

    #Pass in list of users retrieved from Users API
    default_users = []

    def transform(self, itemToTransform: IPublishableCustomView) -> IPublishableCustomView:
        itemToTransform.default_users = self.default_users
        return itemToTransform
```

### Registration

For detailed instructions on registering the transformer, refer to the documentation.

```
plan_builder.transformers.add(CustomViewDefaultUsersTransformer)
```

### Transformer Class

In C#, the transformer class for adding tags is implemented as shown below:

```
public class CustomViewExcludeDefaultUserTransformer(
    ISharedResourcesLocalizer localizer,
    ILogger<CustomViewExcludeDefaultUserTransformer> logger)
    : ContentTransformerBase<IPublishableCustomView>(localizer, logger)
{
    public IList<string> ExcludeUsernames { get; } = new List<string>() { "User1", "User2" };

    private readonly ILogger<CustomViewExcludeDefaultUserTransformer>? _logger = logger;

    public override async Task<IPublishableCustomView?> TransformAsync(IPublishableCustomView itemToTransform, CancellationToken cancel)
    {
        var newDefaultUsers = itemToTransform.DefaultUsers.Where(user => !ExcludeUsernames.Contains(user.Name)).ToList();

        itemToTransform.DefaultUsers = newDefaultUsers;

        _logger?.LogInformation(
            @"Excluding default users {newDefaultUsers}",
            newDefaultUsers);

        return await Task.FromResult(itemToTransform);
    }
}
```

### Registration

To register the transformer in C#, follow the guidance provided in the documentation.

```
_planBuilder.Transformers.Add<CustomViewExcludeDefaultUserTransformer, IPublishableCustomView>();
```

### Dependency Injection

Learn more about dependency injection here.

```
services.AddScoped<CustomViewExcludeDefaultUserTransformer>();
```

---

# Sample: Encrypt Extracts

This sample demonstrates how to encrypt workbook and data source extracts, irrespective of their original state.

Both the Python and C# transformer classes inherit from a base class responsible for the core functionality, then generate versions for `IPublishableWorkbook` and `IPublishableDataSource`.

- Python
- C#

### Transformer Class

To encrypt extracts in Python, you can use the following transformer class:

```
from typing import TypeVar
from tableau_migration import (
    ContentTransformerBase,
    IPublishableWorkbook,
    IPublishableDataSource)

T = TypeVar("T")

class EncryptExtractTransformer(ContentTransformerBase[T]):
    def transform(self, itemToTransform: T) -> T:
        itemToTransform.encrypt_extracts = True

        return itemToTransform

class EncryptExtractTransformerForDataSources(EncryptExtractTransformer[IPublishableDataSource]):
    pass

class EncryptExtractTransformerForWorkbooks(EncryptExtractTransformer[IPublishableWorkbook]):
    pass
```

### Registration

```
plan_builder.transformers.add(EncryptExtractTransformerForDataSources)
plan_builder.transformers.add(EncryptExtractTransformerForWorkbooks)
```

See hook registration for more details.

### Transformer Class

In C#, the transformer class for encrypting extracts is implemented as follows:

```
public class EncryptExtractsTransformer<T> : ContentTransformerBase<T> where T : IContentReference, IFileContent, IExtractContent
{
    private readonly ILogger<IContentTransformer<T>>? _logger;

    public EncryptExtractsTransformer(ISharedResourcesLocalizer localizer, ILogger<IContentTransformer<T>> logger) : base(localizer, logger)
    {
        _logger = logger;
    }

    public override async Task<T?> TransformAsync(T itemToTransform, CancellationToken cancel)
    {
        itemToTransform.EncryptExtracts = true;

        _logger?.LogInformation(
            @"Setting encrypt extract to true for {ContentType} {ContentLocation}",
            typeof(T).Name,
            itemToTransform.Location);

        return await Task.FromResult(itemToTransform);
    }

    public async Task<IPublishableWorkbook?> TransformAsync(IPublishableWorkbook ctx, CancellationToken cancel)
        => await TransformAsync(ctx, cancel);

    public async Task<IPublishableDataSource?> TransformAsync(IPublishableDataSource ctx, CancellationToken cancel)
        => await TransformAsync(ctx, cancel);
}
```

### Registration

To register the transformer in C#, follow the guidance provided in the documentation.

```
_planBuilder.Transformers.Add<EncryptExtractsTransformer<IPublishableDataSource>, IPublishableDataSource>();
_planBuilder.Transformers.Add<EncryptExtractsTransformer<IPublishableWorkbook>, IPublishableWorkbook>();
```

### Dependency Injection

Learn more about dependency injection here.

```
services.AddScoped<EncryptExtractsTransformer<IPublishableDataSource>>();
services.AddScoped<EncryptExtractsTransformer<IPublishableWorkbook>>();
```

---

# Sample: Adding Tags to Content

This sample illustrates how to add a `Migrated` tag to both data sources and workbooks.

Both the Python and C# transformer classes inherit from a base class that handles the core functionality, then create versions for `IPublishableWorkbook` and `IPublishableDataSource`.

- Python
- C#

### Transformer Class

To implement the tag addition in Python, you can utilize the following transformer class:

```
from typing import TypeVar
from tableau_migration import (
    ContentTransformerBase,
    IPublishableDataSource,
    IPublishableWorkbook)
from tableau_migration.migration_content import PyTag

T = TypeVar("T")

class MigratedTagTransformer(ContentTransformerBase[T]):
    def transform(self, itemToTransform: T) -> T:
        tag: str = "Migrated"

        new_tag = PyTag.create(tag)

        current_tags = list(itemToTransform.tags)
        current_tags.append(new_tag)

        itemToTransform.tags = current_tags

        return itemToTransform

class MigratedTagTransformerForDataSources(MigratedTagTransformer[IPublishableDataSource]):
    pass

class MigratedTagTransformerForWorkbooks(MigratedTagTransformer[IPublishableWorkbook]):
    pass
```

### Registration

```
plan_builder.transformers.add(MigratedTagTransformerForDataSources)
plan_builder.transformers.add(MigratedTagTransformerForWorkbooks)
```

See hook registration for more details.

### Transformer Class

In C#, the transformer class for adding tags is implemented as shown below:

```
public class MigratedTagTransformer<T> : ContentTransformerBase<T> where T : IContentReference, IWithTags
{
    private readonly ILogger<IContentTransformer<T>>? _logger;

    public MigratedTagTransformer(ISharedResourcesLocalizer localizer, ILogger<IContentTransformer<T>> logger) : base(localizer, logger)
    {
        _logger = logger;
    }

    public override async Task<T?> TransformAsync(T itemToTransform, CancellationToken cancel)
    {
        var tag = "Migrated";

        // Add the tag to the content item.
        itemToTransform.Tags.Add(new Tag(tag));

        _logger?.LogInformation(
            @"Added ""{Tag}"" tag to {ContentType} {ContentLocation}.",
            tag,
            typeof(T).Name,
            itemToTransform.Location);

        return await Task.FromResult(itemToTransform);
    }

    public async Task<IPublishableWorkbook?> TransformAsync(IPublishableWorkbook ctx, CancellationToken cancel)
        => await TransformAsync(ctx, cancel);

    public async Task<IPublishableDataSource?> TransformAsync(IPublishableDataSource ctx, CancellationToken cancel)
        => await TransformAsync(ctx, cancel);
}
```

### Registration

To register the transformer in C#, follow the guidance provided in the documentation.

```
_planBuilder.Transformers.Add<MigratedTagTransformer<IPublishableDataSource>, IPublishableDataSource>();
_planBuilder.Transformers.Add<MigratedTagTransformer<IPublishableWorkbook>, IPublishableWorkbook>();
```

### Dependency Injection

Learn more about dependency injection here.

```
services.AddScoped<MigratedTagTransformer<IPublishableDataSource>>();
services.AddScoped<MigratedTagTransformer<IPublishableWorkbook>>();
```

---

# Sample: Modify Permissions

This sample demonstrates how to modify permissions that are automatically updated as part of the standard post-publish step the SDK performs. To perform a separate permission update, with full control over the update logic, see the update permissions post-publish hook sample.

Permission transformers are registered for the `IPermissionSet` type and runs for all content types that support permissions, as well as project default permissions for all content types.

- Python
- C#

### Transformer Class

To modify permissions in Python, you can use the following transformer class:

```
from tableau_migration import (
    ContentTransformerBase,
    GranteeType,
    IPermissionSet
)

class ModifyPermissionsTransformer(ContentTransformerBase[IPermissionSet]):
    def transform(self, item_to_transform: IPermissionSet) -> IPermissionSet:
        filtered_grantees = [g for g in item_to_transform.grantee_capabilities if g.grantee_type != GranteeType.GROUP]
        return item_to_transform
```

### Registration

```
plan_builder.transformers.add(ModifyPermissionsTransformer)
```

See hook registration for more details.

### Transformer Class

In C#, the transformer class for modifying permissions is implemented as follows:

```
public class ModifyPermissionsTransformer
    : ContentTransformerBase<IPermissionSet>
{
    public ModifyPermissionsTransformer(ISharedResourcesLocalizer localizer, ILogger<IContentTransformer<IPermissionSet>> logger)
        : base(localizer, logger)
    { }

    public override Task<IPermissionSet?> TransformAsync(IPermissionSet itemToTransform, CancellationToken cancel)
    {
        itemToTransform.GranteeCapabilities = itemToTransform.GranteeCapabilities
            .Where(g => g.GranteeType is GranteeType.Group)
            .ToList();

        return Task.FromResult<IPermissionSet?>(itemToTransform);
    }
}
```

### Registration

To register the transformer in C#, follow the guidance provided in the documentation.

```
_planBuilder.Transformers.Add<ModifyPermissionsTransformer, IPermissionSet>();
```

### Dependency Injection

Learn more about dependency injection here.

```
services.AddScoped<ModifyPermissionsTransformer>();
```

---

# Sample: Adjust 'Start At' to Scheduled Tasks

This sample demonstrates how to adjust the 'Start At' of a Scheduled Task.

Both the Python and C# transformer classes inherit from a base class responsible for the core functionality.

- Python
- C#

### Transformer Class

To adjust the 'Start At' in Python, you can use the following transformer class:

```
from datetime import time
from tableau_migration import (
    ContentTransformerBase,
    ICloudExtractRefreshTask
)

class SimpleScheduleStartAtTransformer(ContentTransformerBase[ICloudExtractRefreshTask]):
    def transform(self, itemToTransform: ICloudExtractRefreshTask) -> ICloudExtractRefreshTask:
        # In this example, the `Start At` time is in the UTC time zone.
        if itemToTransform.schedule.frequency_details.start_at:
            prev_start_at = itemToTransform.schedule.frequency_details.start_at
            # A simple conversion to the EDT time zone.
            itemToTransform.schedule.frequency_details.start_at = time(prev_start_at.hour - 4, prev_start_at.minute, prev_start_at.second, prev_start_at.microsecond);

        return itemToTransform
```

### Registration

```
plan_builder.transformers.add(SimpleScheduleStartAtTransformer)
```

See hook registration for more details.

### Transformer Class

In C#, the transformer class for adjusting the 'Start At' of a given Scheduled Task is implemented as follows:

```
public class SimpleScheduleStartAtTransformer<T>
    : ContentTransformerBase<T>
    where T : IWithSchedule<ICloudSchedule>
{
    private readonly ILogger<IContentTransformer<T>>? _logger;

    public SimpleScheduleStartAtTransformer(
        ISharedResourcesLocalizer localizer,
        ILogger<IContentTransformer<T>> logger)
        : base(
              localizer,
              logger)
    {
        _logger = logger;
    }

    public override async Task<T?> TransformAsync(
        T itemToTransform,
        CancellationToken cancel)
    {
        // In this example, the `Start At` time is in the UTC time zone.
        if (itemToTransform.Schedule.FrequencyDetails.StartAt is not null)
        {
            // A simple conversion to the EDT time zone.
            var updatedStartAt = itemToTransform.Schedule.FrequencyDetails.StartAt.Value.AddHours(-4);

            _logger?.LogInformation(
                @"Adjusting the 'Start At' from {previousStartAt} to {updatedStartAt}.",
                itemToTransform.Schedule.FrequencyDetails.StartAt.Value,
                updatedStartAt);

            itemToTransform.Schedule.FrequencyDetails.StartAt = updatedStartAt;
        }

        return await Task.FromResult(itemToTransform);
    }
}
```

### Registration

To register the transformer in C#, follow the guidance provided in the documentation.

```
_planBuilder.Transformers.Add<SimpleScheduleStartAtTransformer<ICloudExtractRefreshTask>, ICloudExtractRefreshTask>();
```

### Dependency Injection

Learn more about dependency injection here.

```
services.AddScoped(typeof(SimpleScheduleStartAtTransformer<>));
```
