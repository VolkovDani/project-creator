'use client';
import { useEffect, useState } from 'react';
export default function Group({
  id,
  anchorId,
  title,
  summary,
  children,
  defaultOpen = true,
  lazy = false,
  onOpenChange,
}: {
  id: string;
  anchorId?: string;
  title: string;
  summary?: string;
  children: React.ReactNode;
  defaultOpen?: boolean;
  lazy?: boolean;
  onOpenChange?: (open: boolean) => void;
}) {
  const [open, setOpen] = useState(defaultOpen);
  const [restored, setRestored] = useState(false);
  useEffect(() => {
    try {
      const saved = sessionStorage.getItem(`group:${id}`);
      setOpen(saved === null ? defaultOpen : saved === 'true');
    } catch {}
    setRestored(true);
  }, [id, defaultOpen]);
  useEffect(() => {
    if (restored) onOpenChange?.(open);
  }, [open, restored, onOpenChange]);
  return (
    <details
      id={anchorId}
      className="data-group"
      open={open}
      data-restored={String(restored)}
      aria-label={title}
      onToggle={(event) => {
        const value = event.currentTarget.open;
        setOpen(value);
        if (restored)
          try {
            sessionStorage.setItem(`group:${id}`, String(value));
          } catch {}
      }}
    >
      <summary>
        <h3>{title}</h3>
        {summary && <span className="muted">{summary}</span>}
      </summary>
      {(!lazy || (restored && open)) && children}
    </details>
  );
}
