# Evergreen Data Retention Policy

Evergreen has implemented data retention policies to optimize storage usage, reduce costs, and streamline operations. Here are the key retention strategies for different types of data:

### General Retention Policy
- **Live Task and Host Data**: Stored for 365 days. This includes database documents, artifacts, and logs in S3.

### Metadata Retention
- **Task and Host Metadata**: Stored in a data warehouse for 1825 days.
- **Test Results Metadata**: Retained for 730 days.

### Specific Data Retention Policies

#### Task Artifacts
- **S3 Policy**: 
  - Transition to Standard-IA after 30 days.
  - Expire after 365 days.

#### DB Task Artifacts
- **Policy**: 365-day TTL index on the `create_time` field.

#### Logs
- **S3 Storage**: All logs expire after 365 days.

#### Test Results
- **MongoDB**: 365-day TTL index on the `created_at` field.
- **S3**: Expire after 730 days.

#### Versions, Builds, Patches, and Tasks
- **Policy**: 365-day TTL index on the relevant `create_time` field.

#### Hosts/Pods
- **Policy**: 365-day TTL index on the `termination_time` field for applicable hosts.

### Trino Specific Data
- **Retention**: Tasks and hosts data are stored for 1825 days in `mongodatalake-dev-prod-live` and `mongodatalake-dev-prod-staging`.

### General Policy for Trino Data
- **Default Retention**: 1825 days for data in `dev_prod_live` and `dev_prod_staging` schemas. Custom per-dataset policies may be applied as needed.

These policies help manage storage costs and improve operational efficiency while preserving the ability to conduct historical analysis as required.
