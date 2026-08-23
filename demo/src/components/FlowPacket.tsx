import {Easing, interpolate, useCurrentFrame} from 'remotion';
import {clamp, colors, mono} from '../theme';

export const FlowPacket: React.FC<{fromX: number; toX: number; y: number; start: number; end: number; label: string; tone: 'attention' | 'healthy'}> = ({fromX, toX, y, start, end, label, tone}) => {
  const frame = useCurrentFrame();
  return <div style={{position: 'absolute', left: 0, top: 0, opacity: interpolate(frame, [start, start + 4, end - 4, end], [0, 1, 1, 0], clamp), translate: `${interpolate(frame, [start, end], [fromX, toX], {...clamp, easing: Easing.bezier(0.4, 0, 0.2, 1)})}px ${y}px`, zIndex: 20}}>
    <div style={{display: 'flex', alignItems: 'center', gap: 8, padding: '7px 11px', borderRadius: 9, fontFamily: mono, fontSize: 13, fontWeight: 700, color: tone === 'attention' ? '#ffd49d' : '#baf2c8', background: tone === 'attention' ? '#4a2f18' : '#173b25', border: `1px solid ${tone === 'attention' ? '#96632e' : '#36784a'}`, boxShadow: '0 10px 24px #0008'}}>
      <span style={{width: 7, height: 7, borderRadius: 7, background: tone === 'attention' ? colors.orange : colors.green}} />{label}
    </div>
  </div>;
};
