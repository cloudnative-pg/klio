import type {ComponentProps, ComponentType, ReactElement} from "react";
import clsx from "clsx";
import styles from "@site/src/components/HomepageFeatures/styles.module.css";
import Heading from "@theme/Heading";

type FeatureItem = {
    title: string;
    Svg: ComponentType<ComponentProps<'svg'>>;
    description: string;
};

function Feature({title, Svg, description}: FeatureItem): ReactElement<FeatureItem> {
    return (
        <div className={clsx('col col--3')}>
            <div className="text--center">
                <Svg className={styles.featureSvg} role="img"/>
            </div>
            <div className="text--center padding-horiz--md">
                <Heading as="h3">{title}</Heading>
                <p>{description}</p>
            </div>
        </div>
    );
}

export function FeatureList(): ReactElement<null> {
    return (
        <div className="row">
            <Feature
                title={'Multi-tiered storage support'}
                description={"Store your backups on local volumes and relay them to object stores."}
                Svg={require('@site/static/img/undraw_going-up_g8av.svg').default}
            />
            <Feature
                title={'WAL streaming support'}
                description={"Reduce RPO by streaming WALs to the archive."}
                Svg={require('@site/static/img/undraw_season-change_ohe6.svg').default}
            />
            <Feature
                title={'Data security'}
                description={"Encrypt your backups at rest and in transit."}
                Svg={require('@site/static/img/undraw_security_0ubl.svg').default}
            />
            <Feature
                title={'Deduplication'}
                description={"Reduce storage usage and backup time by deduplicating data."}
                Svg={require('@site/static/img/undraw_building-blocks_h5jb.svg').default}
            />
        </div>
    )
}
