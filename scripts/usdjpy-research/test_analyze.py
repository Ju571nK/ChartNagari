import datetime as dt
import unittest
from analyze import simulate, select, stats

UTC=dt.timezone.utc
START=dt.datetime(2026,1,1,tzinfo=UTC)

class AnalysisTests(unittest.TestCase):
    def test_ambiguous_bar_and_spread(self):
        bars=[dict(open_time=(START+dt.timedelta(hours=i)).isoformat(),timeframe='1H',close=100,high=100,low=100) for i in range(22)]
        bars[1].update(high=102,low=98)
        o=dict(entry_time=bars[0]['open_time'],direction='LONG',tp=101.5,sl=99)
        self.assertAlmostEqual(simulate(o,bars,{o['entry_time']:0},1,False)['pips'],149)
        self.assertAlmostEqual(simulate(o,bars,{o['entry_time']:0},1,True)['pips'],-101)
        o.update(direction='SHORT',tp=98.5,sl=101)
        self.assertAlmostEqual(simulate(o,bars,{o['entry_time']:0},3,True)['pips'],-103)

    def test_timeout_and_close_time(self):
        bars=[dict(open_time=(START+dt.timedelta(hours=i*4)).isoformat(),timeframe='4H',close=100,high=100,low=100) for i in range(22)]
        o=dict(entry_time=bars[0]['open_time'],direction='LONG',tp=102,sl=99)
        result=simulate(o,bars,{o['entry_time']:0},2,True)
        self.assertEqual(result['pips'],-2)
        self.assertEqual(result['decision'],START+dt.timedelta(hours=4))
        self.assertEqual(result['exit_time'],START+dt.timedelta(hours=84))

    def test_proxy_no_overlap_cooldown_daily_limit(self):
        trades=[dict(rule='a',score=12,decision=START+dt.timedelta(hours=i),exit_time=START+dt.timedelta(hours=i+2)) for i in range(24)]
        chosen=select(trades,{'ict'},12,{'a':'ict'},True)
        self.assertEqual([t['decision'].hour for t in chosen],[0,4,8])
        self.assertEqual(len(select(trades,{'ict'},16,{'a':'ict'},True)),0)
        self.assertEqual(len(select(trades,{'candlestick'},12,{'a':'ict'},True)),0)

    def test_utc_day_reset_and_priority(self):
        trades=[dict(rule='a',score=8,decision=START,exit_time=START+dt.timedelta(hours=1)),dict(rule='b',score=16,decision=START,exit_time=START+dt.timedelta(hours=1))]
        chosen=select(trades,{'ict'},0,{'a':'ict','b':'ict'},True)
        self.assertEqual(chosen[0]['rule'],'b')
        trades=[dict(rule='a',score=12,decision=START+dt.timedelta(hours=i),exit_time=START+dt.timedelta(hours=i+1)) for i in [0,4,8,12,24]]
        self.assertEqual(len(select(trades,{'ict'},12,{'a':'ict'},True)),4)

    def test_loss_statistics(self):
        s=stats([dict(pips=p,r=r) for p,r in [(10,1),(-20,-2),(-10,-1),(15,1.5)]])
        self.assertEqual(s['net_pips'],-5)
        self.assertAlmostEqual(s['profit_factor'],25/30)
        self.assertEqual(s['max_drawdown_r'],3)
        self.assertEqual(s['max_consecutive_losses'],2)

if __name__=='__main__': unittest.main()
