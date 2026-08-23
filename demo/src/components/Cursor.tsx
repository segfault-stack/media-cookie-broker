import {Easing, interpolate, useCurrentFrame} from 'remotion';
import {clamp, colors, sans} from '../theme';

export const Cursor: React.FC<{start: number; click: number}> = ({start, click}) => {
  const frame = useCurrentFrame();
  return <div style={{position: 'absolute', left: 0, top: 0, zIndex: 50, opacity: interpolate(frame, [start, start + 5, click + 18, click + 23], [0, 1, 1, 0], clamp), translate: `${interpolate(frame, [start, click - 5], [1110, 1042], {...clamp, easing: Easing.bezier(0.2, 0.8, 0.2, 1)})}px ${interpolate(frame, [start, click - 5], [590, 474], {...clamp, easing: Easing.bezier(0.2, 0.8, 0.2, 1)})}px`, scale: interpolate(frame, [click - 2, click, click + 3, click + 7], [1, 0.82, 0.82, 1], clamp)}}>
    <div style={{position: 'absolute', left: -10, top: -10, width: 26, height: 26, borderRadius: 30, border: `2px solid ${colors.blue}`, opacity: interpolate(frame, [click, click + 12], [0.75, 0], clamp), scale: interpolate(frame, [click, click + 12], [0.4, 1.7], clamp)}} />
    <svg width="32" height="39" viewBox="0 0 32 39"><path d="M3 2v28l7.6-7.2 5.7 13.1 5.8-2.7-5.8-12.4H28L3 2Z" fill="#f7fbff" stroke="#101820" strokeWidth="2.3" strokeLinejoin="round" /></svg>
    <div style={{position: 'absolute', top: 37, left: -76, width: 150, textAlign: 'center', fontFamily: sans, fontSize: 12, fontWeight: 800, color: '#c6e2ff', opacity: interpolate(frame, [click + 4, click + 9], [0, 1], clamp)}}>HUMAN CLICK</div>
  </div>;
};
