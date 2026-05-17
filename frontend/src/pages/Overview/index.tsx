import { Empty, Typography } from 'antd';
import { useTranslation } from 'react-i18next';

const { Title } = Typography;

export default function OverviewPage() {
  const { t } = useTranslation();
  return (
    <>
      <Title level={2}>{t('page.overview.title')}</Title>
      <Empty description={t('page.overview.placeholder')} />
    </>
  );
}
