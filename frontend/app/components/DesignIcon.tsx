import sprite from '../../public/design-icons.svg';
import styles from './DesignIcon.module.css';

const symbols = {
  overview: 'grid',
  requirements: 'file-text',
  search: 'search',
  help: 'help-circle',
  classification: 'layers',
  glossary: 'book-open',
  analysis: 'bar-chart-2',
  technical: 'database',
  indexing: 'database',
  projects: 'folder',
  language: 'globe',
  theme: 'moon',
  next: 'chevron-right',
} as const;

export function DesignIcon({
  name,
  className,
}: {
  name: keyof typeof symbols;
  className?: string;
}) {
  const iconClass = name === 'next' ? styles.chevron : styles.icon;
  return (
    <svg
      aria-hidden="true"
      focusable="false"
      className={className ? `${iconClass} ${className}` : iconClass}
      width="20"
      height="20"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <use href={`${sprite.src}#${symbols[name]}`} />
    </svg>
  );
}
