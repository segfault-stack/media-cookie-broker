import {colors, mono, sans} from '../theme';

export const BrokerNode: React.FC<{state: 'healthy17' | 'attention17' | 'healthy18'}> = ({state}) => {
  const attention = state === 'attention17';
  const revision = state === 'healthy18' ? 18 : 17;
  return (
    <div style={{width: 150, padding: '19px 13px 16px', borderRadius: 17, textAlign: 'center', background: '#17202b', border: `1px solid ${attention ? '#8b5f2e' : '#4b5f79'}`, boxShadow: `0 15px 45px ${attention ? '#ff9d2730' : '#6f83ff20'}`, fontFamily: sans}}>
      <div style={{width: 50, height: 50, margin: '0 auto 12px', display: 'grid', placeItems: 'center', borderRadius: 15, color: '#dfdcff', background: 'linear-gradient(145deg,#524c8e,#2d4673)', border: '1px solid #776fae', fontSize: 26}}>🍪</div>
      <div style={{fontSize: 13, fontWeight: 800, color: colors.text, lineHeight: 1.2}}>COOKIE<br />BROKER</div>
      <div style={{fontFamily: mono, color: attention ? colors.orange : colors.green, fontSize: 14, marginTop: 12}}>revision {revision}</div>
      <div style={{fontSize: 12, fontWeight: 750, color: attention ? colors.orange : colors.green, marginTop: 5}}>{attention ? 'refresh_required' : 'Healthy'}</div>
    </div>
  );
};
