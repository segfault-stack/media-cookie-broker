import {Easing, interpolate, useCurrentFrame} from 'remotion';
import {clamp, colors, mono, sans} from '../theme';

export const TerminalCard: React.FC<{state: 'healthy17' | 'failure' | 'healthy18'; revealFailureAt?: number}> = ({state, revealFailureAt = 0}) => {
  const frame = useCurrentFrame();
  const failure = state === 'failure';
  const final = state === 'healthy18';
  return (
    <div style={{width: 390, height: 390, borderRadius: 18, overflow: 'hidden', border: `1px solid ${colors.border}`, background: '#0e151c', boxShadow: '0 22px 60px #0007'}}>
      <div style={{height: 48, display: 'flex', alignItems: 'center', gap: 9, padding: '0 17px', background: '#18222c', borderBottom: `1px solid ${colors.border}`, fontFamily: sans, color: colors.muted, fontSize: 13}}>
        <span style={{width: 10, height: 10, borderRadius: 9, background: '#ff6b6b'}} /><span style={{width: 10, height: 10, borderRadius: 9, background: '#f0bc57'}} /><span style={{width: 10, height: 10, borderRadius: 9, background: '#54ca77'}} />
        <span style={{marginLeft: 9}}>remote consumer</span>
      </div>
      <div style={{padding: '25px 26px', fontFamily: mono, color: '#d9e3ed', fontSize: 17, lineHeight: 1.72}}>
        <div style={{fontFamily: sans, fontSize: 13, textTransform: 'uppercase', letterSpacing: 1.7, color: colors.blue, fontWeight: 800, marginBottom: 15}}>media-worker</div>
        <div><span style={{color: colors.faint}}>$</span> cookie-sync status</div>
        <div style={{marginTop: 14, color: colors.muted}}>scope</div>
        <div style={{color: colors.text}}>youtube/default</div>
        <div style={{marginTop: 14, color: colors.muted}}>{final ? 'current revision' : 'using revision'}</div>
        <div style={{display: 'flex', alignItems: 'center', gap: 12, color: final ? colors.green : colors.text}}>
          revision {final ? 18 : 17}
          {final && <span style={{fontFamily: sans, fontSize: 15, color: colors.green}}>✓</span>}
        </div>
        {failure && <div style={{marginTop: 17, opacity: interpolate(frame, [revealFailureAt, revealFailureAt + 8], [0, 1], {...clamp, easing: Easing.bezier(0.16, 1, 0.3, 1)}), translate: `0 ${interpolate(frame, [revealFailureAt, revealFailureAt + 8], [10, 0], clamp)}px`}}>
          <div style={{color: colors.orange}}>authentication_required</div>
          <div style={{color: colors.muted}}>revision 17</div>
        </div>}
        {final && <div style={{marginTop: 17, padding: '10px 12px', borderRadius: 9, background: colors.greenBg, border: '1px solid #2c6d42', color: '#b2efc2', fontFamily: sans, fontWeight: 700}}>cookies.txt <span style={{color: colors.green}}>✓</span></div>}
      </div>
    </div>
  );
};
