import { Empty, Typography } from 'antd';
import { useTranslation } from 'react-i18next';

const { Title } = Typography;

export default function MetricsPage() {
  const { t } = useTranslation();
  return (
    <>
      <Title level={2}>{t('page.metrics.title')}</Title>
      <Empty description={t('page.metrics.placeholder')} />
    </>
  );
}
