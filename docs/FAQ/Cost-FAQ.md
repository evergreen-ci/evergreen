# Cost FAQ

Evergreen tracks the infrastructure cost of every task across three categories:

| Category | Description                                                                                                                                                                                                     |
| -------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| EC2      | The machine that runs the task. Cost is based on instance type and the task's running time. Costs on static hosts and time spent on host provisioning, host teardown, and task group teardown are not included. |
| EBS      | The disk attached to the EC2 instance. GP3 volumes can be provisioned with higher throughput than the 125 MB/s free tier; the throughput cost above that baseline is also tracked.                              |
| S3       | Object storage where task artifacts and logs are uploaded. Both the upload requests (PUT costs) and the ongoing storage are tracked.                                                                            |

All costs displayed in Evergreen have applicable discounts applied. Discount rates, the finance formula, and other cost parameters are owned by the Financial Planning and Analysis (FP&A) team.

Cost information is shown on the task, version, and patch pages in the Evergreen UI.

- On task pages, the cost is shown once the task completes.
- On version and patch pages, a running total of costs from completed tasks is shown throughout, including child patch costs.

Once all tasks have finished, the full cost breakdown becomes available on each page, including a link to Honeycomb for a more detailed per-component view. Cost data is also available via the REST API. See [How can I view cost data via the REST API?](#how-can-i-view-cost-data-via-the-rest-api).

## What cost fields does Evergreen track?

Cost is tracked at the task level and rolls up to the version and patch level as tasks finish.

**Task-level fields:**

| Category | Subcategory      | Description                                                              |
| -------- | ---------------- | ------------------------------------------------------------------------ |
| EC2      |                  | Runtime cost with discounts applied                                      |
| EBS      | Throughput       | Cost above the 125 MB/s free tier, with discount applied                 |
| EBS      | Storage          | Cost for the attached volume, with discount applied                      |
| S3       | Artifact PUT     | PUT request cost for uploading user artifacts via the `s3.put` command   |
| S3       | Artifact storage | Storage cost for uploaded artifacts using S3 Intelligent Tiering pricing |
| S3       | Log PUT          | PUT request cost for uploading task logs                                 |
| S3       | Log storage      | Storage cost for uploaded logs over their retention period               |

All values reflect discounted costs. For field names and the full cost breakdown, see [How can I view cost data via the REST API?](#how-can-i-view-cost-data-via-the-rest-api).

**Version-level fields:**

Versions aggregate costs across all their tasks as they finish.

**Patch-level fields:**

Patches aggregate costs across all their tasks as they finish and also include child patch costs.

## How is EC2 cost calculated?

EC2 is the machine that runs the task. The cost is calculated when the task finishes, using the task's running time, measured from task start to task completion. Costs on static hosts and time spent on host provisioning, host teardown, and task group teardown are not included.

The discounted cost blends a savings plan rate and the standard on-demand rate using a weighting factor:

```text
number_of_seconds_in_hour = 3,600
cost_per_hour             = (finance_formula * savings_plan_rate + (1 - finance_formula) * on_demand_rate)
adjusted_cost             = runtime_seconds * cost_per_hour / number_of_seconds_in_hour
```

`finance_formula` is a ratio that controls how much of the cost is attributed to MongoDB's savings plan coverage versus the standard AWS list price. The savings plan rate and `finance_formula` are provided by the FP&A team. The on-demand rate is the standard AWS list price for the instance type.

## How is EBS cost calculated?

EBS is the disk attached to the EC2 instance. Two components are tracked: throughput cost and storage cost. Both use us-east-1 rates regardless of where the task runs, and both are calculated when the task finishes.

### EBS throughput cost

GP3 volumes can be provisioned with higher throughput than the 125 MB/s AWS free tier. Only the throughput above that baseline is billable. If a distro has no GP3 mount points, or all volumes are at or below 125 MB/s, the throughput cost is zero.

```text
seconds_in_30_day_month       = 2,592,000
billable_throughput           = total_gp3_throughput_MBps - 125
adjusted_ebs_throughput_cost  = (billable_throughput * 0.04 / seconds_in_30_day_month) * runtime_seconds * (1 - ebs_discount)
```

### EBS storage cost

Storage cost is based on the total size of all attached volumes, prorated to the task's actual runtime.

```text
seconds_in_30_day_month   = 2,592,000
adjusted_ebs_storage_cost = (volume_size_GB * 0.08 / seconds_in_30_day_month) * runtime_seconds * (1 - ebs_discount)
```

The EBS discount rate applies to both throughput and storage and is provided by the FP&A team.

## How is S3 cost calculated?

S3 is the object storage where task artifacts and logs are uploaded. Two types of cost are tracked: PUT request costs (charged per upload request) and storage costs (charged for how long the data is retained).

### Artifact PUT cost

Each file upload makes one or more S3 PUT requests. PUT costs are calculated at the time of upload.

```text
s3_put_price                  = 0.000005  # $5 per million PUT requests (AWS standard rate)
adjusted_s3_artifact_put_cost = artifact_put_requests * s3_put_price * (1 - upload_cost_discount)
```

### Log PUT cost

Task logs are uploaded to S3 as the task runs; each upload uses a single PUT request. The same rate and discount as artifact PUT costs apply.

```text
s3_put_price             = 0.000005  # $5 per million PUT requests (AWS standard rate)
adjusted_s3_log_put_cost = log_put_requests * s3_put_price * (1 - upload_cost_discount)
```

### Storage cost

Artifact and log storage both use S3 Intelligent Tiering pricing, which automatically places objects into tiers based on their access patterns. Storage cost is calculated when the task finishes, using each bucket's S3 expiration lifecycle rule to determine the retention period. To simplify the cost calculations, Evergreen starts counting tier days from the day the objects are uploaded.

| Tier              | Days  | Price per GB-month |
| ----------------- | ----- | ------------------ |
| Standard          | 0–30  | $0.023             |
| Infrequent Access | 30–90 | $0.0125            |
| Archive           | 90+   | $0.004             |

The retention period is read from the bucket's S3 expiration lifecycle rule. If no expiration rule is found, Evergreen falls back to a system default retention period. Separate discounts can be configured for each tier. Costs are only calculated for DevProd-owned buckets.

```text
size_gb          = upload_bytes / 1,073,741,824  # bytes to GB
days_in_standard = min(retention_days, 30)
days_in_ia       = max(0, min(retention_days, 90) - 30)
days_in_archive  = max(0, retention_days - 90)

adjusted_s3_storage_cost = size_gb * (
    (days_in_standard / 30) * 0.023  * (1 - standard_storage_discount) +
    (days_in_ia       / 30) * 0.0125 * (1 - ia_storage_discount)       +
    (days_in_archive  / 30) * 0.004  * (1 - archive_storage_discount)
)
```

## How can I view cost data via the REST API?

Each task, version, and patch page in the Evergreen UI includes an **Open in API** link in the top-right of the metadata panel, which opens the full API response in your browser.

Cost fields are returned on the task, version, and patch endpoints. Discounted fields are prefixed `adjusted_` (for example, `adjusted_ec2_cost`). `on_demand_ec2_cost` is also returned as the undiscounted EC2 rate. EBS and S3 `on_demand_` equivalents are not returned by the API but are available in the Honeycomb cost breakdown.

**Task:** Returns `task_cost` (discounted costs broken down by category) and `s3_usage`, which contains the raw S3 upload metrics (PUT request counts and upload bytes) that Evergreen uses to calculate the S3 cost components in `task_cost`.

```text
GET https://evergreen.mongodb.com/rest/v2/tasks/{task_id}
```

**Version:** Returns `cost` (aggregated discounted cost across all tasks in the version) and `s3_usage` (aggregated artifact and log upload metrics across all tasks). `cost` accumulates as tasks finish and reflects the total cost of all completed tasks so far. Child patch costs are not included; use the patch endpoint to see child patch costs.

```text
GET https://evergreen.mongodb.com/rest/v2/versions/{version_id}
```

**Patch:** Returns `cost` (aggregated discounted cost for the patch's own tasks as well as child patch costs; `child_patches_total_cost` breaks out the child share) and `s3_usage` (aggregated artifact and log upload metrics for the patch's own tasks; child patch S3 usage is not included). The patch ID and version ID are the same value; both endpoints accept the same ID and return the same cost total.

```text
GET https://evergreen.mongodb.com/rest/v2/patches/{patch_id}
```
