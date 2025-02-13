---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "rhcs_hcp_machine_pool Data Source - terraform-provider-rhcs"
subcategory: ""
description: |-
  Machine pool.
---

# rhcs_hcp_machine_pool (Data Source)

Machine pool.



<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `autoscaling` (Attributes) Basic autoscaling options (see [below for nested schema](#nestedatt--autoscaling))
- `cluster` (String) Identifier of the cluster. After the creation of the resource, it is not possible to update the attribute value.
- `name` (String) Name of the machine pool. Must consist of lower-case alphanumeric characters or '-', start and end with an alphanumeric character. After the creation of the resource, it is not possible to update the attribute value.

### Optional

- `auto_repair` (Boolean) Indicates use of autor repair for replica
- `availability_zone` (String) Select the availability zone in which to create a single AZ machine pool for a multi-AZ cluster. After the creation of the resource, it is not possible to update the attribute value.
- `aws_node_pool` (Attributes) AWS settings for node pool (see [below for nested schema](#nestedatt--aws_node_pool))
- `labels` (Map of String) Labels for the machine pool. Format should be a comma-separated list of 'key = value'. This list will overwrite any modifications made to node labels on an ongoing basis.
- `replicas` (Number) The number of machines of the pool
- `subnet_id` (String) Select the subnet in which to create a single AZ machine pool for BYO-VPC cluster. After the creation of the resource, it is not possible to update the attribute value.
- `taints` (Attributes List) Taints for a machine pool. Format should be a comma-separated list of 'key=value'. This list will overwrite any modifications made to node taints on an ongoing basis. (see [below for nested schema](#nestedatt--taints))
- `tuning_configs` (List of String) A list of tuning configs attached to the replica.
- `upgrade_acknowledgements_for` (String) Indicates acknowledgement of agreements required to upgrade the cluster version between minor versions (e.g. a value of "4.12" indicates acknowledgement of any agreements required to upgrade to OpenShift 4.12.z from 4.11 or before).
- `version` (String) Desired version of OpenShift for the machine pool, for example '4.11.0'. If version is greater than the currently running version, an upgrade will be scheduled.

### Read-Only

- `current_version` (String) The currently running version of OpenShift on the machine pool, for example '4.11.0'.
- `id` (String) Unique identifier of the machine pool.
- `status` (Attributes) HCP replica status (see [below for nested schema](#nestedatt--status))

<a id="nestedatt--autoscaling"></a>
### Nested Schema for `autoscaling`

Required:

- `enabled` (Boolean) Enables autoscaling. If `true`, this variable requires you to set a maximum and minimum replicas range using the `max_replicas` and `min_replicas` variables.

Optional:

- `max_replicas` (Number) The maximum number of replicas for autoscaling functionality.
- `min_replicas` (Number) The minimum number of replicas for autoscaling functionality.


<a id="nestedatt--aws_node_pool"></a>
### Nested Schema for `aws_node_pool`

Required:

- `instance_type` (String) Identifier of the machine type used by the nodes, for example `m5.xlarge`. Use the `rhcs_machine_types` data source to find the possible values. After the creation of the resource, it is not possible to update the attribute value.

Optional:

- `tags` (Map of String) Apply user defined tags to all machine pool resources created in AWS. After the creation of the resource, it is not possible to update the attribute value.

Read-Only:

- `instance_profile` (String) Instance profile attached to the replica


<a id="nestedatt--taints"></a>
### Nested Schema for `taints`

Required:

- `key` (String) Taints key
- `schedule_type` (String) Taints schedule type
- `value` (String) Taints value


<a id="nestedatt--status"></a>
### Nested Schema for `status`

Read-Only:

- `current_replicas` (Number) The current number of replicas.
- `message` (String) Message regarding status of the replica
