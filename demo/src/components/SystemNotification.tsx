import {Easing, interpolate, useCurrentFrame} from 'remotion';
import {clamp, colors, sans} from '../theme';

export const SystemNotification: React.FC<{start: number}> = ({start}) => {
  const frame = useCurrentFrame();
  return <div style={{position: 'absolute', right: 35, top: 42, width: 365, padding: '17px 18px', zIndex: 45, borderRadius: 16, border: `1px solid ${colors.border}`, background: '#1d2732f5', boxShadow: '0 22px 60px #0009', fontFamily: sans, opacity: interpolate(frame, [start, start + 8], [0, 1], clamp), translate: `${interpolate(frame, [start, start + 11], [28, 0], {...clamp, easing: Easing.bezier(0.16, 1, 0.3, 1)})}px 0`}}>
    <div style={{display: 'flex', gap: 13, alignItems: 'center'}}>
      <div style={{width: 44, height: 44, borderRadius: 13, background: 'linear-gradient(145deg,#4b6ea7,#342f65)', display: 'grid', placeItems: 'center', fontSize: 22}}>🍪</div>
      <div><div style={{fontWeight: 800, fontSize: 15, color: colors.text}}>Media Cookie Broker</div><div style={{marginTop: 3, fontSize: 14, color: '#dbe5ee'}}>YouTube/default needs authentication</div></div>
    </div>
  </div>;
};
