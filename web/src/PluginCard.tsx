import { useState } from 'react';
import { useTranslation } from 'react-i18next';

export type PluginCardProps = {
	plugin: {
		name: string;
		url: string;
		enabled: boolean;
		symbols: string[];
		min_score: number;
		direction_filter: '' | 'LONG' | 'SHORT';
		secret: string;
	};
	stats?: {
		plugin_id: string;
		submitted: number;
		filled: number;
		rejected: number;
		last_failure_msg: string;
	};
	onEdit: () => void;
	onDelete: () => Promise<void>;
	onToggleEnabled: (next: boolean) => Promise<void>;
};

export default function PluginCard({ plugin, stats, onEdit, onDelete, onToggleEnabled }: PluginCardProps) {
	const { t } = useTranslation();
	const [confirming, setConfirming] = useState(false);
	const [deleting, setDeleting] = useState(false);

	const hasFailure = !!stats?.last_failure_msg;
	const classes = [
		'plugin-card',
		!plugin.enabled ? 'plugin-card--disabled' : '',
		hasFailure ? 'plugin-card--has-failure' : '',
	].filter(Boolean).join(' ');

	const confirmDelete = async () => {
		setDeleting(true);
		try { await onDelete(); } finally { setDeleting(false); setConfirming(false); }
	};

	return (
		<div className={classes}>
			<label>
				<input
					type="checkbox"
					checked={plugin.enabled}
					onChange={e => { void onToggleEnabled(e.target.checked); }}
				/>
			</label>
			<span className="plugin-name">{plugin.name}</span>
			<span className="plugin-url">{plugin.url}</span>
			<span className="plugin-stats">
				{stats
					? `24h: ${stats.filled} filled / ${stats.rejected} rejected`
					: t('execution.no_activity')}
			</span>
			{hasFailure && (
				<span className="plugin-failure-tooltip" title={stats!.last_failure_msg}>!</span>
			)}
			<button onClick={onEdit}>{t('common.edit')}</button>
			<button onClick={() => setConfirming(true)}>{t('common.delete')}</button>

			{confirming && (
				<div role="dialog" className="modal-backdrop" style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000 }}>
					<div className="modal" style={{ background: 'var(--bg)', padding: 24, borderRadius: 8, minWidth: 320 }}>
						<p>{t('execution.confirm_delete_plugin', { name: plugin.name })}</p>
						<div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end', marginTop: 16 }}>
							<button onClick={() => setConfirming(false)}>{t('common.cancel')}</button>
							<button onClick={() => void confirmDelete()} disabled={deleting}>{t('common.confirm')}</button>
						</div>
					</div>
				</div>
			)}
		</div>
	);
}
