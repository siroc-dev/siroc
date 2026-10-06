import { Skeleton } from "antd";

export function PageSkeleton({
  cards = 0,
  rows = 8,
  bare = false,
}: {
  cards?: number;
  rows?: number;
  bare?: boolean;
}) {
  const body = (
    <>
      {bare ? null : <Skeleton.Input active className="page-skel-title" />}
      {cards > 0 ? (
        <div className="page-skel-cards">
          {Array.from({ length: cards }, (_, i) => (
            <div key={i} className="page-skel-card">
              <Skeleton active title={false} paragraph={{ rows: 2 }} />
            </div>
          ))}
        </div>
      ) : null}
      <div className="page-skel-table">
        {Array.from({ length: rows }, (_, i) => (
          <Skeleton.Input key={i} active block className="page-skel-row" />
        ))}
      </div>
    </>
  );
  if (bare) {
    return (
      <div className="page-skel" aria-busy="true">
        {body}
      </div>
    );
  }
  return (
    <div className="cp-page page-skel" aria-busy="true">
      {body}
    </div>
  );
}

export function BootSkeleton() {
  return (
    <div className="boot-skel" aria-busy="true">
      <div className="boot-skel-side">
        <Skeleton.Input active className="page-skel-title" />
        {Array.from({ length: 12 }, (_, i) => (
          <Skeleton.Input key={i} active block className="page-skel-row" />
        ))}
      </div>
      <div className="boot-skel-main">
        <PageSkeleton cards={4} rows={6} />
      </div>
    </div>
  );
}
