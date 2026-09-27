# Retry policy review

Operator: The current immediate retries amplify outages.
Lead: Decision: production requests will use three attempts with exponential backoff starting at 200ms.
Operator: Agreed. The runbook needs to reflect that policy.
