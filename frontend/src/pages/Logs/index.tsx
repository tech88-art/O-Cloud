import { Empty, Typography } from 'antd';
import { useTranslation } from 'react-i18next';

const { Title } = Typography;

export default function LogsPage() {
  const { t } = useTranslation();
  return (
    <>
      <Title level={2}>{t('page.logs.title')}</Title>
      <Empty description={t('page.logs.placeholder')} />
    </>
  );
}
