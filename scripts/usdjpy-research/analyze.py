#!/usr/bin/env python3
"""Evaluate an offline replay; no live configuration or notification access."""
import argparse, collections, csv, datetime as dt, hashlib, json, math, random
from pathlib import Path


def date(s):
    return dt.datetime.fromisoformat(s.replace('Z', '+00:00'))


def simulate(o, bars, index, spread, conservative):
    i = index[o['entry_time']]
    mid = bars[i]['close']
    long = o['direction'] == 'LONG'
    risk_pips = abs(mid - o['sl']) / .01
    for k in range(i + 1, i + 21):
        b = bars[k]
        tp_hit = b['high'] >= o['tp'] if long else b['low'] <= o['tp']
        sl_hit = b['low'] <= o['sl'] if long else b['high'] >= o['sl']
        if sl_hit and (conservative or not tp_hit):
            exit_price = o['sl']
        elif tp_hit:
            exit_price = o['tp']
        elif k == i + 20:
            exit_price = b['close']
        else:
            continue
        pips = (exit_price - mid) / .01 * (1 if long else -1) - spread
        return dict(o, pips=pips, r=pips/risk_pips,
                    decision=date(bars[i]['open_time']) + duration(bars[i]['timeframe']),
                    exit_time=date(b['open_time']) + duration(b['timeframe']))
    raise AssertionError('No exit')


def duration(tf):
    return dt.timedelta(hours=1 if tf == '1H' else 4)


def stats(trades):
    p = [x['pips'] for x in trades]
    rs = [x['r'] for x in trades]
    profit, loss = sum(x for x in p if x > 0), -sum(x for x in p if x < 0)
    equity = peak = dd = 0
    streak = max_streak = 0
    for x in rs:
        equity += x
        peak = max(peak, equity)
        dd = max(dd, peak-equity)
        streak = streak+1 if x < 0 else 0
        max_streak = max(max_streak, streak)
    return dict(trades=len(p), win_rate=sum(x > 0 for x in p)/len(p) if p else 0,
                profit_factor=profit/loss if loss else None, net_pips=sum(p),
                avg_pips=sum(p)/len(p) if p else 0, net_r=sum(rs), max_drawdown_r=dd,
                worst_trade_pips=min(p) if p else 0, max_consecutive_losses=max_streak)


def select(outcomes, methods, score, lookup, proxy):
    kept = []
    last = {}
    counts = collections.Counter()
    free = dt.datetime.min.replace(tzinfo=dt.timezone.utc)
    for o in sorted(outcomes, key=lambda o: (o['decision'], -o['score'], o['rule'])):
        if lookup[o['rule']] not in methods or o['score'] < score:
            continue
        if proxy:
            if o['decision'] < free:
                continue
            day = o['decision'].date().isoformat()
            if counts[day] >= 3 or (o['rule'] in last and o['decision']-last[o['rule']] < dt.timedelta(hours=4)):
                continue
            counts[day] += 1
            last[o['rule']] = o['decision']
            free = o['exit_time']
        kept.append(o)
    return kept


def weekly_bootstrap(trades, start, end):
    # Resample calendar-week blocks, including weeks without trades.
    weeks = {}
    day = start
    while day < end:
        weeks.setdefault(day.isocalendar()[:2], 0.0)
        day += dt.timedelta(days=1)
    for trade in trades:
        weeks[trade['decision'].isocalendar()[:2]] += trade['r']
    values = list(weeks.values())
    rng = random.Random(571)
    totals = sorted(sum(rng.choice(values) for _ in values) for _ in range(10000))
    return dict(weeks=len(values), samples=10000, seed=571,
                net_r_95_interval=[totals[249], totals[9749]])


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--input', default='tmp/usdjpy-research/replay.json')
    ap.add_argument('--output', default='tmp/usdjpy-research/comparison.json')
    args = ap.parse_args()
    j = json.loads(Path(args.input).read_text())
    lookup = {r['Name']: r['Methodology'] for r in j['rules_config']['Rules']}
    bars4 = j['bars']['4H']
    start = date(bars4[200]['open_time']) + duration('4H')
    end = date(bars4[-21]['open_time']) + duration('4H')
    cutoff = (start + (end-start)*.7).replace(hour=0, minute=0, second=0, microsecond=0)
    groups = {'forex_default': {'ict','smc','general_ta','candlestick'},
              'ict_smc': {'ict','smc'}, 'general_ta': {'general_ta'},
              'candlestick': {'candlestick'},
              'ict_smc_candlestick': {'ict','smc','candlestick'},
              'ta_candlestick': {'general_ta','candlestick'}}
    rows, diagnostics, uncertainty = [], {}, {}
    for tf in ['1H','4H']:
        bars = j['bars'][tf]
        index = {b['open_time']: i for i,b in enumerate(bars)}
        eligible = [o for o in j['results'][tf]['outcomes']
                    if index[o['entry_time']] + 20 < len(bars)
                    and start <= date(o['entry_time'])+duration(tf) < end]
        # Require a full exit horizon before the train/holdout boundary.
        train_limit = max(i for i,b in enumerate(bars) if date(b['open_time'])+duration(tf) < cutoff) - 20
        diagnostics[tf] = dict(bars=len(bars), first=bars[0]['open_time'], last=bars[-1]['open_time'],
                               raw_outcomes=len(j['results'][tf]['outcomes']), eligible_outcomes=len(eligible))
        # Validate the independent TP-first replay against the application's engine.
        for o in eligible:
            x = simulate(o,bars,index,1,False)
            assert math.isclose(x['pips'],o['pnl_pips'],abs_tol=1e-8)
        for conservative in [False,True]:
            for spread in [1,2,3]:
                enriched = [simulate(o,bars,index,spread,conservative) for o in eligible]
                for segment in ['train','holdout']:
                    subset = [o for o in enriched if
                              (index[o['entry_time']] <= train_limit if segment=='train' else o['decision'] >= cutoff)]
                    for group, methods in groups.items():
                        for threshold in [0,4,8,12,16]:
                            for proxy in [False, True]:
                                selected = select(subset,methods,threshold,lookup,proxy)
                                rows.append(dict(timeframe=tf, segment=segment, group=group, threshold=threshold,
                                                 spread_pips=spread, exit_policy='sl_first' if conservative else 'tp_first',
                                                 mode='one_position_proxy' if proxy else 'raw_rule_diagnostic',
                                                 **stats(selected)))
        # Bootstrap only fixed baseline and train-selected candidate; no holdout tuning.
        candidates=[r for r in rows if r['timeframe']==tf and r['segment']=='train'
                    and r['mode']=='one_position_proxy' and r['exit_policy']=='sl_first'
                    and r['spread_pips']==1 and r['trades']>=30]
        best=max(candidates,key=lambda r:(r['net_r'],-r['max_drawdown_r']),default=None)
        holdout=[simulate(o,bars,index,1,True) for o in eligible if date(o['entry_time'])+duration(tf)>=cutoff]
        uncertainty[tf]={}
        for label, group, threshold in [('baseline','forex_default',12),
                                       ('train_selected',best['group'],best['threshold'])] if best else [('baseline','forex_default',12)]:
            selected=select(holdout,groups[group],threshold,lookup,True)
            uncertainty[tf][label]=dict(group=group,threshold=threshold,
                                       **weekly_bootstrap(selected,cutoff,end))
        # Individual-rule holdout diagnostics, SL-first, 1 pip; overlapping trades.
        enriched=[simulate(o,bars,index,1,True) for o in eligible if date(o['entry_time'])+duration(tf) >= cutoff]
        diagnostics[tf]['rules']={r:stats([o for o in enriched if o['rule']==r]) for r in sorted(set(o['rule'] for o in enriched))}
    report=dict(start=start.isoformat(), end=end.isoformat(), cutoff=cutoff.isoformat(),
                train_ratio=.7, boundary_embargo_bars=20, diagnostics=diagnostics, rows=rows, uncertainty=uncertainty,
                replay_sha256=hashlib.sha256(Path(args.input).read_bytes()).hexdigest())
    Path(args.output).write_text(json.dumps(report,indent=2))
    csv_path=Path(args.output).with_suffix('.csv')
    with csv_path.open('w',newline='') as f:
        writer=csv.DictWriter(f,fieldnames=list(rows[0]),lineterminator="\n");writer.writeheader();writer.writerows(rows)
    print(json.dumps({k:report[k] for k in ['start','end','cutoff']},indent=2))
    for tf in ['1H','4H']:
        train=[r for r in rows if r['timeframe']==tf and r['segment']=='train' and r['mode']=='one_position_proxy'
               and r['exit_policy']=='sl_first' and r['spread_pips']==1 and r['trades']>=30]
        best=max(train,key=lambda r:(r['net_r'],-r['max_drawdown_r']),default=None)
        print(tf,'best TRAIN candidate',json.dumps(best))
        if best:
            print('HOLDOUT',json.dumps([r for r in rows if all(r[k]==best[k] for k in ['timeframe','group','threshold','mode','exit_policy']) and r['segment']=='holdout']))
        print('CURRENT PROFILE HOLDOUT',json.dumps([r for r in rows if r['timeframe']==tf and r['group']=='forex_default' and r['threshold']==12 and r['segment']=='holdout' and r['mode']=='one_position_proxy' and r['exit_policy']=='sl_first']))

if __name__=='__main__':
    main()
