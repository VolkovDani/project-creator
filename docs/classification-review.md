# Reviewing requirement classifications

[Русская версия](classification-review.ru.md)

Open **Classification** in the project navigation or expand **Classification** on a requirement card. Read the original text and check **Meaning** and **Form** separately. The class guide includes distinguishing features, examples and counterexamples for both axes.

Select **Review classification**, choose a value for each axis, then **Save classification**. Either axis can be **Undetermined**. Closing the dialog discards the selection. When the source text or decision changes while you are editing, check the current text before applying your selections to the new version. The card identifies the text version of a previous decision.

## Model evidence and manual decisions

The card displays the effective decision, its state, its source and the model name when recorded. Expand the alternatives to inspect the three highest-ranked alternatives per group and their uncalibrated model scores. Expand decision details for recorded model and prompt provenance. The review-only model can leave both axes undetermined and still provide alternatives. A record can also need review without alternatives, for example when its input exceeds the model limit.

An explicit rerun on a confirmed record keeps the user's decision. Open **Decision history** to compare later model evidence, earlier decisions, the source text version and the decision revision. Scroll to the end of the history or press **Load more** to see the next 10 decisions. With automatic classification disabled, manual review and history remain available.

## Explaining a classification

Open **Independent AI assessment** on the classification card. The section appears once the classifier has saved a proposed label or ranked alternatives for the current text version. A later manual confirmation or correction keeps it available. A manual decision alone, a queued or failed check, and a check that produced no labels or alternatives do not qualify.

Choose **Target meaning** and **Target form** independently, then select **Explain classification**. You can choose categories different from the saved labels. The local model reads the original text, the saved proposal and the effective decision when available. It assesses each axis against the chosen category and quotes the source for criteria it finds. For example, a user story is assessed for role, action and value. Acceptance evidence is optional for that form. The result can include clarifying questions, suggested wording and limitations. To use a wording suggestion, copy it into the requirement editor, review it and save the edit yourself. Requesting an explanation changes neither the text nor the classification.

You can cancel a request. If the text, decision or model proposal changes, review the current card before requesting another assessment. After a text edit, the classifier must first produce a proposal for that version. If the model is busy, retry later. Identical requests can reuse one of the 64 results held in server memory. Reuse depends on the workspace, source and decision revisions, selected categories, language, taxonomy and explanation model profile. Restarting the server clears the cache.

The server uses the configured, digest-pinned local Ollama summary generator if available. Otherwise it uses the configured, digest-pinned local Ollama general generator. The card reports when the local provider is unavailable, the request exceeds the model’s context limit, or the model’s response fails validation. It also distinguishes a timeout, a cancelled request and a failed request to the server. Only successful explanations enter the cache. Source text stays local throughout the request.

## Review queues

On the project classification page, filter by meaning, form and status. **Needs review** opens that queue. The group counts describe the whole project. Select a group to filter the records. Scroll to the end of the records or press **Load more** to append the next 20. Classification updates refresh individual visible cards and aggregate counts.

Use **Classify pending records** for existing records and **Retry failed classifications** to retry failures. The progress panel shows queued, running and failed counts. You can request or retry a single record from its classification card on this page. To confirm several saved model suggestions, select their checkboxes across the loaded records and choose **Confirm selected**. The checkbox appears for current suggestions with a known meaning and form. Review-only alternatives and partially undetermined decisions require individual review. Loading more records keeps the selection. Changing filters clears it. Before each save, the app checks the selected labels, source version and decision revision. The result lists each confirmed, changed or failed record.

[Classification rules](requirement-classification.md)
