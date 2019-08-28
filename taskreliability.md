*Note to self:* [TOC](https://github.com/evergreen-ci/evergreen/wiki/REST-V2-Usage)
----------

TaskReliability
----------

Task Reliability success scores are aggregated task execution statistics for a given project. Statistics can be grouped by time period (days) and by task, variant, distro combinations.

The score is based on the lower bound value of a [Binomial proportion confidence interval](https://en.wikipedia.org/wiki/Binomial_proportion_confidence_interval).

In this case, the equation is a [Wilson score interval](https://en.wikipedia.org/wiki/Binomial_proportion_confidence_interval#Wilson%20score%20interval%20with%20continuity%20correction):

![Wilson score interval with continuity correction](https://wikimedia.org/api/rest_v1/media/math/render/svg/5dd0452ea2018ec009a6294014a8d7068ef7e39b)

> In statistics, a binomial proportion confidence interval is a confidence interval for the probability of success calculated from the outcome of a series of success–failure experiments (Bernoulli trials). In other words, a binomial proportion confidence interval is an interval estimate of a success probability p when only the number of experiments n and the number of successes nS are known.

The advantage of using a confidence interval of this sort is that the computed value takes the number of test into account. The lower the number of test, the greater the margin of error. This results in a lower success rate score for the cases where there are fewer test results.

During the evaluation of this algorithm, 22 consecutive test passes are required before a success  score of .85 is reached (with a significance level / α of _0.05_).

Objects
~~~~~~~

.. list-table:: **TaskReliability**
   :widths: 25 10 55
   :header-rows: 1

   * - Name
     - Type
     - Description
   * - ``task_name``
     - string
     - Name of the task the test ran under.
   * - ``variant``         
     - string         
     - Name of the build variant the task ran on. Omitted if the grouping does not include the build variant.
   * - ``distro``             
     - string         
     - Identifier of the distro that the task ran on. Omitted if the grouping does not include the distro.
   * - ``date``
     - string
     - The start date ("YYYY-MM-DD" UTC day) of the period the statistics cover.
   * - ``num_success``
     - int
     - The number of times the task was successful during the target period.
   * - ``num_failed``
     - int
     - The number of times the task failed during the target period.
   * - ``num_total``
     - int
     - The number of times the task ran during the target period.
   * - ``num_timeout``
     - int
     - The number of times the task ended on a timeout during the target period.
   * - ``num_test_failed``
     - int
     - The number of times the task failed with a failure of type `test` during the target period.
   * - ``num_system_failed``
     - int
     - The number of times the task failed with a failure of type `system` during the target period.
   * - ``num_setup_failed``
     - int
     - The number of times the task failed with a failure of type `setup` during the target period.
   * - ``avg_duration_success``
     - float
     - The average duration, in seconds, of the tasks that passed during the target period.
   * - ``success_rate``
     - float
     - The success rate score calculated over the time span, grouped by time period and distro, variant or task. The value ranges from 0.0 (total failure) to 1.0 (total success).

Endpoints
~~~~~~~~~

Fetch the Task Reliability score for a project
``````````````````````````````

::

 GET /projects/<project_id>/task_reliability

Returns a paginated list of task reliability scores associated with a specific project filtered and grouped according to the query parameters.

.. list-table:: **Parameters**
   :widths: 15 20 55
   :header-rows: 1

   * - Name
     - Type
     - Description
   * - ``after_date``
     - string
     - The start date (included) of the targeted time interval. The format is "YYYY-MM-DD". The date is UTC. Defaults to today.
   * - ``before_date``
     - string
     - The end date (included) of the targeted time interval. The format is "YYYY-MM-DD". The date is UTC. Defaults to today.
   * - ``group_num_days``
     - int
     - Optional. Indicates that the statistics should be aggregated by groups of ``group_num_days`` days. The first group will start on the nearest first date greater than ``after_date``, the last group will start on ``before_date`` - ``group_num_days``` days. Defaults to 1.
   * - ``requesters``
     - []string or comma separated strings
     - Optional. The requesters that triggered the task execution. Accepted values are ``mainline``, ``patch``, ``trigger``, and ``adhoc``. Defaults to ``mainline``.
   * - ``tasks``
     - []string or comma separated strings
     - The tasks to include in the statistics.
   * - ``variants``
     - []string or comma separated strings
     - Optional. The build variants to include in the statistics.
   * - ``distros``
     - []string or comma separated strings
     - Optional. The distros to include in the statistics.
   * - ``group_by``
     - string
     - Optional. How to group the results. Accepted values are ``task``, ``task_variant``, and ``task_variant_distro``. By default the results are grouped by task.
   * - ``sort``
     - string
     - Optional. The order in which the results are returned. Accepted values are ``earliest`` and ``latest``. Defaults to ``latest``.
   * - ``start_at``
     - string
     - Optional. The identifier of the task stats to start at in the pagination
   * - ``limit``
     - int
     - Optional. The number of task stats to be returned per page of pagination. Defaults to 1000.
