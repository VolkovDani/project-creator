'use client';
import Link from 'next/link';
import { useRouter } from 'next/navigation';
import { useEffect, useRef, useState } from 'react';
import contract from '../../../../contracts/project.json';
import { query } from '../../../lib/client';
import { CreateProjectDocument } from '../../../lib/generated';
import { t } from '../../../lib/i18n';
import { type FieldError, type Input, validate } from '../../../lib/validation';
import { InterfacePreferences, useLocale } from '../../components/LocaleProvider';

const empty: Input = { name: '', goal: '', description: '' };
export default function NewProject() {
  const { locale } = useLocale();
  const router = useRouter();
  const [input, setInput] = useState<Input>(empty);
  const [errors, setErrors] = useState<FieldError[]>([]);
  const [busy, setBusy] = useState(false);
  const [submitFailure, setSubmitFailure] = useState(false);
  const active = useRef(true);
  useEffect(() => {
    active.current = true;
    return () => {
      active.current = false;
    };
  }, []);
  const fields = ['name', 'goal', 'description'] as const;
  async function submit(e: React.FormEvent) {
    e.preventDefault();
    setSubmitFailure(false);
    const checked = validate(contract, input);
    setErrors(checked.errors);
    if (checked.errors.length) return;
    setBusy(true);
    try {
      const data = await query(CreateProjectDocument, { input: checked.normalized });
      if (!active.current) return;
      const result = data.createProject;
      if (result.fieldErrors.length) {
        setErrors(
          result.fieldErrors.map((x) => ({
            path: x.path,
            code: x.code,
            messageKey: x.messageKey,
            params: Object.fromEntries(x.params.map((p) => [p.key, p.value])),
          })),
        );
        return;
      }
      if (!result.project) throw Error('missing project');
      router.push(`/projects/${encodeURIComponent(result.project.id)}`);
    } catch {
      if (active.current) setSubmitFailure(true);
    } finally {
      if (active.current) setBusy(false);
    }
  }
  return (
    <main>
      <div className="top">
        <Link href="/">{t(locale, 'app.back')}</Link>
        <InterfacePreferences />
      </div>
      <section>
        <h1>{t(locale, 'app.create')}</h1>
        <form onSubmit={submit} noValidate>
          {fields.map((field) => {
            const error = errors.find((x) => x.path[0] === 'input' && x.path[1] === field);
            return (
              <label key={field} htmlFor={`project-${field}`}>
                {t(locale, `app.${field}`)}
                {field === 'description' ? (
                  <textarea
                    id={`project-${field}`}
                    value={input[field]}
                    onChange={(e) => setInput({ ...input, [field]: e.target.value })}
                    aria-invalid={!!error}
                  />
                ) : (
                  <input
                    id={`project-${field}`}
                    value={input[field]}
                    onChange={(e) => setInput({ ...input, [field]: e.target.value })}
                    aria-invalid={!!error}
                  />
                )}
                {error && (
                  <span className="error" role="alert">
                    {t(locale, error.messageKey, error.params)}
                  </span>
                )}
              </label>
            );
          })}
          {submitFailure && (
            <p className="error" role="alert">
              {t(locale, 'app.submitError')}
            </p>
          )}
          {errors.some(
            (x) => x.path[0] !== 'input' || !fields.includes(x.path[1] as (typeof fields)[number]),
          ) && (
            <p className="error" role="alert">
              {t(locale, 'app.validationUnknown')}
            </p>
          )}
          <button type="submit" disabled={busy}>
            {t(locale, busy ? 'app.creating' : 'app.create')}
          </button>
        </form>
      </section>
    </main>
  );
}
