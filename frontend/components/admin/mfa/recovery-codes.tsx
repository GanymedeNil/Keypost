'use client';

import { Download } from 'lucide-react';
import { useI18n } from '@/lib/i18n/provider';
import { BRAND } from '@/lib/brand';
import { Button, CopyButton } from '@/components/kit';

/** The one-time list of recovery codes, with copy and download. */
export function RecoveryCodes({ codes }: { codes: string[] }) {
  const { t } = useI18n();
  const text = codes.join('\n');

  function download() {
    const blob = new Blob([`${BRAND.name} recovery codes\n\n${text}\n`], { type: 'text/plain' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${BRAND.name.toLowerCase()}-recovery-codes.txt`;
    a.click();
    URL.revokeObjectURL(url);
  }

  return (
    <div className="space-y-3">
      <ul className="grid grid-cols-2 gap-2 rounded-lg border border-border bg-muted/60 p-3 font-mono text-sm" aria-label="recovery codes">
        {codes.map((c) => (
          <li key={c} className="text-center tracking-wide">
            {c}
          </li>
        ))}
      </ul>
      <div className="flex gap-2">
        <CopyButton value={text} variant="secondary" size="md" showText label={t('mfa.recovery.copyAll')} />
        <Button variant="secondary" icon={<Download className="size-4" />} onClick={download}>
          {t('mfa.recovery.download')}
        </Button>
      </div>
    </div>
  );
}
