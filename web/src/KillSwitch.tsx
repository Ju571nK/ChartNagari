import { useState } from 'react';
import { useTranslation } from 'react-i18next';

type Props = {
	killed: boolean;
	killedAt: string | null;
	onToggle: () => Promise<void>;
};

function formatKilledAt(iso: string | null): string {
	if (!iso) return '';
	try {
		return new Date(iso).toISOString().replace('T', ' ').slice(0, 19) + ' UTC';
	} catch {
		return iso;
	}
}

export default function KillSwitch({ killed, killedAt, onToggle }: Props) {
	const { t } = useTranslation();
	const [modalOpen, setModalOpen] = useState(false);
	const [busy, setBusy] = useState(false);

	const confirm = async () => {
		setBusy(true);
		try { await onToggle(); } finally { setBusy(false); setModalOpen(false); }
	};

	return (
		<div className="kill-switch">
			{killed ? (
				<div role="banner" className="kill-banner" style={{ background: 'var(--danger)', color: '#fff', padding: '12px' }}>
					{t('execution.killed_banner')} — {t('execution.last_killed')}: {formatKilledAt(killedAt)}
					<button onClick={() => setModalOpen(true)} disabled={busy} style={{ marginLeft: '12px' }}>
						{t('execution.reenable')}
					</button>
				</div>
			) : (
				<div className="kill-bar" style={{ padding: '8px 0' }}>
					<button
						onClick={() => setModalOpen(true)}
						style={{ background: 'var(--danger)', color: '#fff', padding: '8px 16px', border: 'none', borderRadius: 4 }}
					>
						{t('execution.kill_switch')}
					</button>
				</div>
			)}

			{modalOpen && (
				<div role="dialog" className="modal-backdrop" style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000 }}>
					<div className="modal" style={{ background: 'var(--bg)', padding: 24, borderRadius: 8, minWidth: 320 }}>
						<p>{killed ? t('execution.confirm_reenable') : t('execution.confirm_kill')}</p>
						<div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end', marginTop: 16 }}>
							<button onClick={() => setModalOpen(false)}>{t('common.cancel')}</button>
							<button onClick={confirm} disabled={busy}>{t('common.confirm')}</button>
						</div>
					</div>
				</div>
			)}
		</div>
	);
}
