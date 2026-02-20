# Evergreen Data Retention Reference

## Overview

- **Primary Storage (MongoDB, S3):**
  - **Live Task & Host Data:** Retained for **365 days**

- **Data Warehouse (Trino):**
  - **Task & Host Metadata:** Retained for **1825 days** (5 years)
  - **Test Results Metadata:** Retained for **1825 days** (5 years)

---

### Detailed Retention by Data Type

#### Tasks & Hosts

- **MongoDB:** Retained for **365 days** from `create_time` or `termination_time` (as applicable).
- **Trino (Data Warehouse):** Retained for **1825 days** for tasks and hosts data.

#### Artifacts

Artifact retention rates depend on which bucket they are stored in. Different projects have different retention rates for their default bucket, and projects may use more than one bucket.

#### Logs

- **S3 Logs for failed tasks:** deleted after **180 days**.
- **S3 Logs for successful tasks:** deleted after **90 days** (coming soon, please see [DEVPROD-20335](https://jira.mongodb.org/browse/DEVPROD-20335)).

#### Test Results

- **MongoDB Test Results:** Expire after **365 days** based on `created_at`.
- **S3 Test Results:** Deleted after **730 days** (2 years).

#### Versions, Builds, Patches, and Tasks

- **MongoDB:** Expire after **365 days** based on `create_time`.

#### Hosts

- **MongoDB:** Expire after **365 days** based on `termination_time` if applicable.

---

### Additional Notes

- **Data Warehouse (Trino):**
  - Default retention for data in `dev_prod_live` and `dev_prod_staging` is **1825 days**.
  - Custom per-dataset policies may be applied as necessary.

- **Lifecycle Management:**
  - Automated TTL (Time-to-Live) indices and S3 lifecycle rules ensure smooth transitions from live storage to deletion, optimizing cost and storage usage.
