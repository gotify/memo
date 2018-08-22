import Button from 'material-ui/Button';
import Grid from 'material-ui/Grid';
import Typography from 'material-ui/Typography';
import React, {SFC} from 'react';

interface IProps {
    title: string;
    buttonTitle?: string;
    fButton?: VoidFunction;
    buttonDisabled?: boolean;
    maxWidth?: number;
    hideButton?: boolean;
}

const DefaultPage: SFC<IProps> = ({
    title,
    buttonTitle,
    fButton,
    buttonDisabled = false,
    maxWidth = 700,
    hideButton,
    children,
}) => (
    <main style={{margin: '0 auto', maxWidth}}>
        <Grid container spacing={24}>
            <Grid item xs={12} style={{display: 'flex'}}>
                <Typography variant="display1" style={{flex: 1}}>
                    {title}
                </Typography>
                {hideButton ? null : (
                    <Button
                        variant="raised"
                        color="primary"
                        disabled={buttonDisabled}
                        onClick={fButton}>
                        {buttonTitle}
                    </Button>
                )}
            </Grid>
            {children}
        </Grid>
    </main>
);
export default DefaultPage;
