import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

export type GlobalConfig = {
	max_dispatched: number;
	dedup_window: string;
	symbol_map: Record<string, Record<string, string>>;
};

type Props = {
	config: GlobalConfig;
	onSave: (partial: GlobalConfig) => Promise<void>;
	onServerError: Record<string, string> | null;
};

type Row = { symbol: string; map: { broker: string; ticker: string }[] };

function toRows(m: GlobalConfig['symbol_map']): Row[] {
	return Object.entries(m).map(([symbol, brokers]) => ({
		symbol,
		map: Object.entries(brokers).map(([broker, ticker]) => ({ broker, ticker })),
	}));
}

function fromRows(rows: Row[]): GlobalConfig['symbol_map'] {
	const out: GlobalConfig['symbol_map'] = {};
	for (const r of rows) {
		const sym = r.symbol.trim().toUpperCase();
		if (!sym) continue;
		out[sym] = {};
		for (const { broker, ticker } of r.map) {
			if (broker.trim() && ticker.trim()) out[sym][broker.trim()] = ticker.trim();
		}
	}
	return out;
}

export default function GlobalConfigForm({ config, onSave, onServerError }: Props) {
	const { t } = useTranslation();
	const [maxDispatched, setMaxDispatched] = useState(config.max_dispatched);
	const [dedupWindow, setDedupWindow] = useState(config.dedup_window);
	const [rows, setRows] = useState<Row[]>(toRows(config.symbol_map));
	const [busy, setBusy] = useState(false);

	useEffect(() => {
		setMaxDispatched(config.max_dispatched);
		setDedupWindow(config.dedup_window);
		setRows(toRows(config.symbol_map));
	}, [config]);

	const dirty = useMemo(() =>
		maxDispatched !== config.max_dispatched
		|| dedupWindow !== config.dedup_window
		|| JSON.stringify(fromRows(rows)) !== JSON.stringify(config.symbol_map),
	[maxDispatched, dedupWindow, rows, config]);

	const save = async () => {
		setBusy(true);
		try {
			await onSave({ max_dispatched: maxDispatched, dedup_window: dedupWindow, symbol_map: fromRows(rows) });
		} finally {
			setBusy(false);
		}
	};

	const discard = () => {
		setMaxDispatched(config.max_dispatched);
		setDedupWindow(config.dedup_window);
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
					value={dedupWindow}
					onChange={e => setDedupWindow(e.target.value)}
					style={{ display: 'block', width: 120 }}
				/>
				{onServerError?.dedup_window && (
					<span className="error" style={{ color: 'var(--danger)', fontSize: '0.85em' }}>
						{onServerError.dedup_window}
					</span>
				)}
			</label>

			<details open style={{ marginBottom: 16 }}>
				<summary>{t('execution.symbol_map')}</summary>
				<div style={{ paddingTop: 12 }}>
					{rows.map((row, i) => (
						<div key={i} className="symbol-map-row" style={{ display: 'flex', gap: 6, alignItems: 'center', marginBottom: 6, flexWrap: 'wrap' }}>
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
							{row.map.map((bt, j) => (
								<span key={j} style={{ display: 'flex', gap: 4 }}>
									<input
										placeholder="broker"
										value={bt.broker}
										onChange={e => {
											const next = [...rows];
											next[i] = { ...row, map: row.map.map((x, k) => k === j ? { ...x, broker: e.target.value } : x) };
											setRows(next);
										}}
										style={{ width: 80 }}
									/>
									<input
										placeholder="ticker"
										value={bt.ticker}
										onChange={e => {
											const next = [...rows];
											next[i] = { ...row, map: row.map.map((x, k) => k === j ? { ...x, ticker: e.target.value } : x) };
											setRows(next);
										}}
										style={{ width: 100 }}
									/>
								</span>
							))}
							<button onClick={() => {
								const next = [...rows];
								next[i] = { ...row, map: [...row.map, { broker: '', ticker: '' }] };
								setRows(next);
							}}>{t('execution.add_broker')}</button>
							<button onClick={() => setRows(rows.filter((_, k) => k !== i))}>{t('execution.remove_row')}</button>
						</div>
					))}
					<button onClick={() => setRows([...rows, { symbol: '', map: [{ broker: '', ticker: '' }] }])}>{t('execution.add_row')}</button>
				</div>
			</details>

			<div style={{ display: 'flex', gap: 8, justifyContent: 'flex-end' }}>
				<button onClick={discard} disabled={!dirty}>{t('common.discard')}</button>
				<button onClick={() => void save()} disabled={!dirty || busy}>{t('common.save')}</button>
			</div>
		</div>
	);
}
