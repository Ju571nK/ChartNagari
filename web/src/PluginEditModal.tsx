import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Plugin } from './ExecutionTab';

type Props = {
	plugin: Plugin | null;
	existingNames: string[];
	onSave: (p: Plugin) => Promise<void>;
	onCancel: () => void;
};

const EMPTY: Plugin = {
	name: '', url: '', enabled: true, symbols: [], min_score: 0, direction_filter: '', secret: '',
};

function genHex32(): string {
	const buf = new Uint8Array(32);
	crypto.getRandomValues(buf);
	return Array.from(buf).map(b => b.toString(16).padStart(2, '0')).join('');
}

export default function PluginEditModal({ plugin, existingNames, onSave, onCancel }: Props) {
	const { t } = useTranslation();
	const isCreate = plugin == null || !plugin.name;
	const [form, setForm] = useState<Plugin>(plugin ?? EMPTY);
	const [errors, setErrors] = useState<Record<string, string>>({});
	const [toast, setToast] = useState<string | null>(null);
	const [showSecret, setShowSecret] = useState(false);
	const [busy, setBusy] = useState(false);

	useEffect(() => {
		setForm(plugin ?? EMPTY);
		setErrors({});
	}, [plugin]);

	const validateName = (): string => {
		if (!isCreate) return '';
		if (!form.name.trim()) return t('execution.err_name_required');
		if (existingNames.includes(form.name)) return t('execution.err_name_exists');
		return '';
	};
	const validateURL = (): string =>
		/^https?:\/\//.test(form.url) ? '' : t('execution.err_url');
	const validateMinScore = (): string =>
		form.min_score >= 0 ? '' : t('execution.err_min_score');
	const validateSecret = (): string =>
		(isCreate && !form.secret) ? t('execution.err_secret_required') : '';

	const runValidators = (): Record<string, string> => {
		const e: Record<string, string> = {};
		const n = validateName(); if (n) e.name = n;
		const u = validateURL(); if (u) e.url = u;
		const m = validateMinScore(); if (m) e.min_score = m;
		const s = validateSecret(); if (s) e.secret = s;
		return e;
	};

	const onGenerate = async () => {
		const hex = genHex32();
		setForm(f => ({ ...f, secret: hex }));
		setShowSecret(true);
		try {
			await navigator.clipboard.writeText(hex);
			setToast(t('execution.secret_copied'));
		} catch {
			setToast(t('execution.secret_copy_manual'));
		}
	};

	const onSubmit = async () => {
		const e = runValidators();
		setErrors(e);
		if (Object.keys(e).length > 0) return;
		setBusy(true);
		try { await onSave(form); } finally { setBusy(false); }
	};

	return (
		<div role="dialog" className="modal-backdrop" style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.72)', display: 'flex', alignItems: 'center', justifyContent: 'center', zIndex: 1000 }}>
			<div className="modal plugin-edit-modal" style={{ background: 'var(--bg)', padding: 24, borderRadius: 8, minWidth: 400, maxWidth: 600 }}>
				<label style={{ display: 'block', marginBottom: 12 }}>
					{t('execution.field_name')}
					<input
						aria-label={t('execution.field_name')}
						value={form.name}
						readOnly={!isCreate}
						onChange={e => setForm(f => ({ ...f, name: e.target.value }))}
						onBlur={() => setErrors(p => ({ ...p, name: validateName() }))}
						style={{ display: 'block', width: '100%' }}
					/>
					{errors.name && <span className="error" style={{ color: 'var(--danger)', fontSize: '0.85em' }}>{errors.name}</span>}
				</label>

				<label style={{ display: 'block', marginBottom: 12 }}>
					{t('execution.field_url')}
					<input
						aria-label={t('execution.field_url')}
						value={form.url}
						onChange={e => setForm(f => ({ ...f, url: e.target.value }))}
						onBlur={() => setErrors(p => ({ ...p, url: validateURL() }))}
						style={{ display: 'block', width: '100%' }}
					/>
					{errors.url && <span className="error" style={{ color: 'var(--danger)', fontSize: '0.85em' }}>{errors.url}</span>}
				</label>

				<label style={{ display: 'block', marginBottom: 12 }}>
					{t('execution.field_secret')}
					<div style={{ display: 'flex', gap: 6 }}>
						<input
							aria-label={t('execution.field_secret')}
							type={showSecret ? 'text' : 'password'}
							value={form.secret}
							placeholder={isCreate ? '' : t('execution.secret_placeholder')}
							onChange={e => setForm(f => ({ ...f, secret: e.target.value }))}
							style={{ flex: 1 }}
						/>
						<button type="button" onClick={() => setShowSecret(s => !s)}>{showSecret ? t('common.hide') : t('common.show')}</button>
						<button type="button" onClick={() => void onGenerate()}>{t('execution.generate')}</button>
					</div>
					{errors.secret && <span className="error" style={{ color: 'var(--danger)', fontSize: '0.85em' }}>{errors.secret}</span>}
				</label>

				<label style={{ display: 'block', marginBottom: 12 }}>
					{t('execution.field_min_score')}
					<input
						aria-label={t('execution.field_min_score')}
						type="number"
						value={form.min_score}
						onChange={e => setForm(f => ({ ...f, min_score: parseFloat(e.target.value) || 0 }))}
						onBlur={() => setErrors(p => ({ ...p, min_score: validateMinScore() }))}
						style={{ display: 'block', width: 120 }}
					/>
					{errors.min_score && <span className="error" style={{ color: 'var(--danger)', fontSize: '0.85em' }}>{errors.min_score}</span>}
				</label>

				<label style={{ display: 'block', marginBottom: 12 }}>
					{t('execution.field_symbols')}
					<input
						aria-label={t('execution.field_symbols')}
						value={form.symbols.join(', ')}
						onChange={e => setForm(f => ({ ...f, symbols: e.target.value.split(',').map(s => s.trim().toUpperCase()).filter(Boolean) }))}
						style={{ display: 'block', width: '100%' }}
					/>
				</label>

				<label style={{ display: 'block', marginBottom: 12 }}>
					{t('execution.field_direction')}
					<select
						value={form.direction_filter}
						onChange={e => setForm(f => ({ ...f, direction_filter: e.target.value as Plugin['direction_filter'] }))}
						style={{ display: 'block' }}
					>
						<option value="">{t('common.both')}</option>
						<option value="LONG">LONG</option>
						<option value="SHORT">SHORT</option>
					</select>
				</label>

				<label style={{ display: 'flex', gap: 6, alignItems: 'center', marginBottom: 16 }}>
					<input
						type="checkbox"
						checked={form.enabled}
						onChange={e => setForm(f => ({ ...f, enabled: e.target.checked }))}
					/>
					{t('execution.field_enabled')}
				</label>

				<div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
					<button onClick={onCancel}>{t('common.cancel')}</button>
					<button onClick={() => void onSubmit()} disabled={busy}>{t('common.save')}</button>
				</div>

				{toast && (
					<div role="status" className="toast" style={{ marginTop: 12, padding: 8, background: 'var(--slate)', borderRadius: 4 }}>
						{toast}
					</div>
				)}
			</div>
		</div>
	);
}
