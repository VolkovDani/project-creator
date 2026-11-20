# Requirement classification

[Русская версия](requirement-classification.ru.md)

Rule version 2 adds `test_case` to the representation axis. Meaning and form are independent. Mixed or ambiguous records can have an unknown label on one axis and a known label on the other. Keep the full original text.

| Code | Definition | Example | Counterexample |
| --- | --- | --- | --- |
| `business_requirement` | Business objective | Reduce waiting time by half | Show the queue |
| `stakeholder_requirement` | Stakeholder need | The dispatcher needs to see urgent orders | Save a timestamp |
| `functional_requirement` | System behavior | Save the selected interval | Finish migration by May |
| `non_functional_requirement` | Quality attribute | Respond within one second | Send a receipt |
| `transition_requirement` | Temporary rollout need | Import existing orders before launch | Keep an order history permanently |
| `business_rule` | Domain policy | Only members qualify for discounts | Render the amount in bold |
| `constraint` | Imposed implementation restriction | Use the existing local database | Reduce waiting time |
| `statement` | Declarative assertion or directive | Save the selected interval | Given … when … then … |
| `user_story` | Role, need and purpose | As a clerk I want a queue to select the next order | The system exports a file |
| `use_case` | Actor goal and flow with possible alternatives | Submit order, validate, accept or return for correction | The order is pending |
| `scenario` | Concrete sequence in context | A dispatcher closes an empty queue, reopens it and sees zero orders | Respond within one second |
| `acceptance_criterion` | Verifiable condition for acceptance | Reopening an empty queue must show zero orders | Improve usability |
| `test_case` | Repeatable check with preconditions or test data, steps and an expected result | Start with no orders in queue A. Close and reopen queue A. Check that its order count is 0. | Reopening an empty queue must show zero orders |
| `other` | Recognizable content outside the categories | Meeting note (semantic axis) | Unclear fragment |
| `unknown` | Insufficient evidence for one label | Faster (unspecified object) | A clearly specified export action |

The three empty-queue examples describe the same system behavior (`functional_requirement`). The scenario recounts what the dispatcher does and sees. The acceptance criterion sets the condition for accepting the behavior. The test case tells a tester how to reproduce and check it.

New manual confirmations record rules version 2. Earlier decisions retain their original labels and rules version in history.

Existing records start as `unclassified`. The `confirmRequirementClassification` operation requires the source version and current decision revision. Editing the source makes its previous decision `stale`; history retains the original status. Filters return the latest revision’s types together with its status. The API and UI share the `classifying` state. Alternative scores belong to the model. Automatic classification depends on benchmark #50.
