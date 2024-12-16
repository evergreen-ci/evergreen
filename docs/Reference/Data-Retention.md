
# Evergreen Data Retention Reference

### Overview
- **Primary Storage (MongoDB, S3):**  
  - **Live Task & Host Data:** Retained for **365 days**
  
- **Data Warehouse (Trino):**  
  - **Task & Host Metadata:** Retained for **1825 days** (5 years)  
  - **Test Results Metadata:** Retained for **1825 days** (5 years)

---

### Detailed Retention by Data Type

**Tasks & Hosts**
- **MongoDB:** Retained for **365 days** from `create_time` or `termination_time` (as applicable).  
- **Trino (Data Warehouse):** Retained for **1825 days** for tasks and hosts data.

**Artifacts**
- **S3:**  
  - Transitioned to Standard-IA after **30 days**  
  - Permanently deleted after **365 days**  
- **MongoDB (Task Artifacts):** Expire after **365 days** based on `create_time`.

**Logs**
- **S3 Logs:** Deleted after **365 days**.

**Test Results**
- **MongoDB Test Results:** Expire after **365 days** based on `created_at`.  
- **S3 Test Results:** Deleted after **730 days** (2 years).

**Versions, Builds, Patches, and Tasks**
- **MongoDB:** Expire after **365 days** based on `create_time`.

**Hosts/Pods**
- **MongoDB:** Expire after **365 days** based on `termination_time` if applicable.

---

### Additional Notes

- **Data Warehouse (Trino):**  
  - Default retention for data in `dev_prod_live` and `dev_prod_staging` is **1825 days**.  
  - Custom per-dataset policies may be applied as necessary.

- **Lifecycle Management:**  
  - Automated TTL (Time-to-Live) indices and S3 lifecycle rules ensure smooth transitions from live storage to deletion, optimizing cost and storage usage.

