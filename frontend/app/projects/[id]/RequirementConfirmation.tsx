'use client';
import { type Locale, t } from '../../../lib/i18n';
import ConfirmationDialog from '../../components/ConfirmationDialog';

export default function RequirementConfirmation({
  action,
  text,
  locale,
  onCancel,
  onConfirm,
}: {
  action: 'archive' | 'delete';
  text: string;
  locale: Locale;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const deleting = action === 'delete';
  const characters = Array.from(text);
  return (
    <ConfirmationDialog
      title={t(locale, deleting ? 'requirements.purgeQuestion' : 'requirements.archiveQuestion')}
      confirmLabel={t(locale, deleting ? 'requirements.purge' : 'requirements.confirmArchive')}
      destructive={deleting}
      locale={locale}
      onCancel={onCancel}
      onConfirm={onConfirm}
    >
      <p className="user-content">
        {characters.slice(0, 180).join('')}
        {characters.length > 180 ? '…' : ''}
      </p>
      <p>{t(locale, deleting ? 'requirements.purgeHint' : 'requirements.archiveHint')}</p>
    </ConfirmationDialog>
  );
}
