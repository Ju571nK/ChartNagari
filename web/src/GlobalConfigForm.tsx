import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

export type GlobalConfig = {
	max_dispatched: number;
	dedup_window_sec: number;
	symbol_map?: Record<string, string>;
};

type Props = {
	config: GlobalConfig;
	onSave: (partial: GlobalConfig) => Promise<void>;
	onServerError: Record<string, string> | null;
};

type Row = { symbol: string; ticker: string };

function toRows(m: Record<string, string> | undefined): Row[] {
	return Object.entries(m ?? {}).map(([symbol, ticker]) => ({ symbol, ticker }));
}

function fromRows(rows: Row[]): Record<string, string> {
	const out: Record<string, string> = {};
	for (const r of rows) {
		const sym = r.symbol.trim().toUpperCase();
		if (sym && r.ticker.trim()) out[sym] = r.ticker.trim();
	}
	return out;
}

export default function GlobalConfigForm({ config, onSave, onServerError: _onServerError }: Props) {
	const { t } = useTranslation();
	const [maxDispatched, setMaxDispatched] = useState(config.max_dispatched);
	const [dedupWindowSec, setDedupWindowSec] = useState(config.dedup_window_sec);
	const [rows, setRows] = useState<Row[]>(toRows(config.symbol_map));
	const [busy, setBusy] = useState(false);

	useEffect(() => {
		setMaxDispatched(config.max_dispatched);
		setDedupWindowSec(config.dedup_window_sec);
		setRows(toRows(config.symbol_map));
	}, [config]);

	const dirty = useMemo(() =>
		maxDispatched !== config.max_dispatched
		|| dedupWindowSec !== config.dedup_window_sec
		|| JSON.stringify(fromRows(rows)) !== JSON.stringify(config.symbol_map ?? {}),
	[maxDispatched, dedupWindowSec, rows, config]);

	const save = async () => {
		setBusy(true);
		try {
			await onSave({ max_dispatched: maxDispatched, dedup_window_sec: dedupWindowSec, symbol_map: fromRows(rows) });
		} finally {
			setBusy(false);
		}
	};

	const discard = () => {
		setMaxDispatched(config.max_dispatched);
		setDedupWindowSec(config.dedup_window_sec);
		setRows(toRows(config.symbol_map));
	};

	return (
		<div className="global-config-form">
			<label style={{ display: 'block', marginBottom: 12 }}>
				{t('execution.max_dispatched')}
				<input
					aria-label={t('execution.max_dispatched')}
					type="number"
					value={maxDispatched}
					onChange={e => setMaxDispatched(parseInt(e.target.value, 10) || 0)}
					style={{ display: 'block', width: 120 }}
				/>
			</label>

			<label style={{ display: 'block', marginBottom: 12 }}>
				{t('execution.dedup_window')}
				<input
					aria-label={t('execution.dedup_window')}
					type="number"
					min={0}
					value={dedupWindowSec}
					onChange={e => setDedupWindowSec(parseInt(e.target.value, 10) || 0)}
					style={{ display: 'block', width: 120 }}
				/>
			</label>

			<details open style={{ marginBottom: 16 }}>
				<summary>{t('execution.symbol_map')}</summary>
				<div style={{ paddingTop: 12 }}>
					{rows.map((row, i) => (
						<div key={i} className="symbol-map-row" style={{ display: 'flex', gap: 6, alignItems: 'center', marginBottom: 6 }}>
							<input
								aria-label="symbol"
								value={row.symbol}
								placeholder="symbol"
								onChange={e => {
									const next = [...rows];
									next[i] = { ...row, symbol: e.target.value };
									setRows(next);
								}}
								style={{ width: 100 }}
							/>
							<input
								aria-label="ticker"
								value={row.ticker}
								placeholder="ticker"
								onChange={e => {
									const next = [...rows];
									next[i] = { ...row, ticker: e.target.value };
									setRows(next);
								}}
								style={{ width: 120 }}
							/>
							<button onClick={() => setRows(rows.filter((_, k) => k !== i))}>{t('execution.remove_row')}</button>
						</div>
					))}
					<button onClick={() => setRows([...rows, { symbol: '', ticker: '' }])}>{t('execution.add_row')}</button>
				</div>
			</details>

			<div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
				<button onClick={discard} disabled={!dirty}>{t('common.discard')}</button>
				<button onClick={() => void save()} disabled={!dirty || busy}>{t('common.save')}</button>
			</div>
		</div>
	);
}
