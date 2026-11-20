# Reviewing requirement classifications

[Русская версия](classification-review.ru.md)

Open **Classification** in the project navigation or expand **Classification** on a requirement card. Read the original text and check **Meaning** and **Form** separately. The class guide includes distinguishing features, examples and counterexamples for both axes.

Select **Review classification**, choose a value for each axis, then **Save classification**. Either axis can be **Undetermined**. Closing the dialog discards the selection. When the source text or decision changes while you are editing, check the current text before applying your selections to the new version. The card identifies the text version of a previous decision.

## Model evidence and manual decisions

The card displays the effective decision, its state, its source and the model name when recorded. Expand the alternatives to inspect the ranked labels and their uncalibrated model scores. Expand decision details for recorded model and prompt provenance. The review-only model can leave both axes undetermined and still provide alternatives. A record can also need review without alternatives, for example when its input exceeds the model limit.

An explicit rerun on a confirmed record keeps the user's decision. Open **Decision history** to compare later model evidence, earlier decisions, the source text version and the decision revision. With automatic classification disabled, manual review and history remain available.

## Review queues

On the project classification page, filter by meaning, form and status. **Needs review** opens that queue. The group counts describe the whole project. Select a group to filter the records. Records are paginated. Classification updates refresh individual visible cards and aggregate counts.

Use **Classify pending records** for existing records and **Retry failed classifications** to retry failures. The progress panel shows queued, running and failed counts. You can request or retry a single record from its classification card on this page. To confirm several saved model suggestions, select their checkboxes on the current page and choose **Confirm selected**. The checkbox appears for current suggestions with a known meaning and form. Review-only alternatives and partially undetermined decisions require individual review. Changing filters or pages clears the selection. Before each save, the app checks the selected labels, source version and decision revision. The result lists each confirmed, changed or failed record.

[Classification rules](requirement-classification.md)
