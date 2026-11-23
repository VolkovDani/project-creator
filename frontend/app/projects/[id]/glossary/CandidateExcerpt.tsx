import type { Candidate } from '../CandidateDefinition';
import styles from './CandidateExcerpt.module.css';

export default function CandidateExcerpt({ candidate }: { candidate: Candidate }) {
  return (
    <>
      <span className={candidate.beforeTruncated ? styles.fadeStart : undefined}>
        {candidate.before}
      </span>
      <mark>{candidate.abbreviation}</mark>
      <span className={candidate.afterTruncated ? styles.fadeEnd : undefined}>
        {candidate.after}
      </span>
    </>
  );
}
