import { BackHeader } from '@/components/BackHeader';
import { Button } from '@/components/Sub';
import React, { useEffect, useState } from 'react';
import { history, useParams } from 'umi';
import styles from './index.less';
import { Modal } from 'antd-mobile';
import { JoinModal } from '@/components/PopupModal/join';
import { FullLoading } from '@/components/Loading';
import { ApiGetTradingByID, ITradingCompetitionResp } from '@/apis/trading';
import moment from 'moment';
import { get4SwapUrl, getAuthUrl, getMixSwapUrl, USDT_ASSET_ID } from '@/apis/http';
import { changeTheme } from '@/assets/ts/tools';
import { Icon } from '@/components/Icon';

export default function Page() {
  const { id } = useParams<{ id: string }>();
  const [showModal, setShowModal] = useState(false);
  const [isLoaded, setIsLoaded] = useState(false);
  const [pageData, setPageData] = useState<ITradingCompetitionResp>();

  useEffect(() => {
    initPage().then(() => {
      changeTheme('#D75150');
      let body = document.getElementsByTagName('body')[0];
      body.style.backgroundColor = '#B5312F';
    });
    return () => {
      changeTheme('#fff');
    };
  }, []);
  const initPage = async () => {
    const data = await ApiGetTradingByID(id);
    setPageData(data);
    setIsLoaded(true);
  };

  return (
    <div className={`safe-view ${styles.container}`}>
      <BackHeader
        name=""
        isWhite
        action={
          <>
            <Icon className={styles.action} i="ic_rank" onClick={() => history.push(`/trading/rank/${id}`)} />
            {/* <Icon className={styles.action} i='ic_help' onClick={() => { }} /> */}
          </>
        }
      />
      <div className={styles.head}>
        <h1 className={styles.title}>{pageData?.trading_competition.title}</h1>
        <h3 className={styles.tips}>{pageData?.trading_competition.tips}</h3>
        <img className={styles.head_bg} src={require('@/assets/img/active/trading/reward.png')} alt="" />
      </div>

      <div className={styles.content}>
        <div className={styles.item}>
          <div className={styles.item_title}>????????????</div>
          <p className={styles.item_desc}>{pageData?.trading_competition.rules}</p>
        </div>
        <div className={styles.item}>
          <div className={styles.item_title}>????????????</div>
          <p className={styles.item_desc}>
            {moment(pageData?.trading_competition.start_at).format('YYYY/MM/DD')} ~ {moment(pageData?.trading_competition.end_at).format('YYYY/MM/DD')}
          </p>
        </div>
        <div className={styles.item}>
          <div className={styles.item_title}>????????????</div>
          <p className={styles.item_desc} dangerouslySetInnerHTML={{ __html: pageData?.trading_competition.reward || '' }}></p>
        </div>
      </div>

      {!pageData || pageData.status === '1' ? (
        <Button className={styles.btn} onClick={() => (location.href = getAuthUrl({ returnTo: '', hasSnapshots: true, hasAssets: true }))}>
          ????????????
        </Button>
      ) : (
        <>
          <Button
            onClick={() => {
              setShowModal(true);
              document.getElementsByTagName('body')[0].style.backgroundColor = '#fff';
            }}
            className={styles.btn}
          >
            ?????? {pageData.asset.symbol}
          </Button>
          <span className={styles.check} onClick={() => history.push(`/trading/rank/${id}`)}>
            ????????????
          </span>
        </>
      )}
      <Modal
        visible={showModal}
        animationType="slide-up"
        popup
        onClose={() => {
          setShowModal(false);
          setTimeout(() => {
            document.getElementsByTagName('body')[0].style.backgroundColor = '#B5312F';
          }, 200);
        }}
      >
        <JoinModal
          modalProp={{
            title: '?????? ' + pageData?.asset.symbol,
            desc: `??????????????? MixSwap ?????? 4swap ??? USDT???BTC???ETH ?????????????????? ${pageData?.asset.symbol} ????????????????????????`,
            descStyle: 'blank',
            icon_url: pageData?.asset.icon_url,
            button: '?????? MixSwap ??????',
            buttonAction: () => (location.href = getMixSwapUrl(USDT_ASSET_ID, pageData?.asset.asset_id || '')),
            tips: '?????? 4swap ??????',
            tipsStyle: 'blank',
            tipsAction: () => (location.href = get4SwapUrl(USDT_ASSET_ID, pageData?.asset.asset_id || '')),
            isAirdrop: true,
          }}
        />
      </Modal>
      {!isLoaded && <FullLoading mask />}
    </div>
  );
}
