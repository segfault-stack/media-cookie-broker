import {Easing, interpolate, useCurrentFrame} from 'remotion';
import {clamp, colors, sans} from '../theme';
import {StatusPill} from './StatusPill';

export const ExtensionPopup: React.FC<{state: 'healthy17' | 'attention17' | 'healthy18'; buttonPressed?: boolean}> = ({state, buttonPressed = false}) => {
  const frame = useCurrentFrame();
  const attention = state === 'attention17';
  const revision = state === 'healthy18' ? 18 : 17;
  return (
    <div style={{height: 'calc(100% - 50px)', padding: '23px 25px', background: 'radial-gradient(circle at 100% 0,#223b55 0,#111820 55%)', fontFamily: sans}}>
      <div style={{display: 'flex', alignItems: 'center', gap: 13}}>
        <div style={{width: 45, height: 45, borderRadius: 13, display: 'grid', placeItems: 'center', background: 'linear-gradient(145deg,#426696,#302b62)', boxShadow: '0 9px 22px #0007', fontSize: 23}}>🍪</div>
        <div><div style={{fontSize: 10, letterSpacing: 1.8, fontWeight: 850, color: '#8fc4ff'}}>MEDIA COOKIE BROKER</div><div style={{fontSize: 23, lineHeight: 1.2, fontWeight: 800, color: colors.text, marginTop: 3}}>Publisher status</div></div>
      </div>
      <div style={{marginTop: 18, fontSize: 12, padding: '8px 10px', borderRadius: 8, color: '#a8edba', background: '#173b27', border: '1px solid #27643b'}}>Broker connected · spanning control plane</div>
      <div style={{marginTop: 15, padding: '17px 17px 16px', borderRadius: 13, background: colors.panelRaised, borderLeft: `4px solid ${attention ? colors.orange : colors.green}`, boxShadow: '0 8px 20px #0004'}}>
        <div style={{fontSize: 19, fontWeight: 800, color: colors.text}}>YouTube / default</div>
        <div style={{marginTop: 10}}>
          {attention ? <>
            <StatusPill tone="attention">Authentication requires attention</StatusPill>
            <div style={{fontSize: 14, color: '#ffc981', marginTop: 9, fontWeight: 700}}>1 consumer report · revision 17</div>
          </> : <StatusPill tone="healthy">Healthy · revision {revision}</StatusPill>}
        </div>
        <div style={{fontSize: 12, color: colors.muted, marginTop: 12}}>{attention ? 'Last report: just now' : 'Publisher snapshot available'}</div>
        <div style={{marginTop: 14, height: 45, display: 'grid', placeItems: 'center', borderRadius: 9, color: '#f5f9ff', background: 'linear-gradient(135deg,#397dd1,#2866b2)', border: '1px solid #64a4f5', fontWeight: 800, fontSize: 15, scale: buttonPressed ? interpolate(frame, [15, 18, 21], [1, 0.96, 1], {...clamp, easing: Easing.bezier(0.2, 0.8, 0.2, 1)}) : 1, boxShadow: buttonPressed && frame >= 18 ? '0 0 0 5px #64a4f522' : 'none'}}>Refresh session</div>
      </div>
      <div style={{fontSize: 12, color: colors.muted, lineHeight: 1.45, marginTop: 14}}>Interactive recovery starts only after you select <span style={{color: '#d9e7f5'}}>Refresh session</span>.</div>
    </div>
  );
};
