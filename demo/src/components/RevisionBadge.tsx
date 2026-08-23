import {Easing, interpolate, useCurrentFrame} from 'remotion';
import {clamp, colors, mono, sans} from '../theme';

export const RevisionBadge: React.FC = () => {
  const frame = useCurrentFrame();
  return <div style={{position: 'absolute', left: 498, top: 160, width: 284, padding: '16px 18px', borderRadius: 14, background: '#18222df5', border: `1px solid ${colors.border}`, boxShadow: '0 18px 50px #0008', textAlign: 'center', opacity: interpolate(frame, [0, 7, 34, 44], [0, 1, 1, 0], clamp), translate: `0 ${interpolate(frame, [0, 8], [12, 0], {...clamp, easing: Easing.bezier(0.16, 1, 0.3, 1)})}px`}}>
    <div style={{fontFamily: sans, fontSize: 11, letterSpacing: 1.5, color: colors.muted, fontWeight: 800}}>FRESH PUBLICATION</div>
    <div style={{display: 'flex', justifyContent: 'center', alignItems: 'center', gap: 14, marginTop: 8, fontFamily: mono, fontSize: 18}}><span style={{color: colors.muted, textDecoration: 'line-through'}}>revision 17</span><span style={{color: colors.blue}}>→</span><span style={{color: colors.green, fontWeight: 800}}>revision 18</span></div>
  </div>;
};
