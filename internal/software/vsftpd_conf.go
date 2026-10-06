package software

// vsftpdConfig is /etc/vsftpd.conf. check_shell is off because extra FTP
// logins, and accounts with SSH turned off, use /usr/sbin/nologin. With the
// default check, vsftpd answers 530 Login incorrect before it reads the password.
func vsftpdConfig(pasv string) string {
	conf := `listen=YES
listen_ipv6=NO
anonymous_enable=NO
local_enable=YES
write_enable=YES
local_umask=022
dirmessage_enable=YES
use_localtime=YES
xferlog_enable=YES
connect_from_port_20=YES
chroot_local_user=YES
allow_writeable_chroot=YES
check_shell=NO
userlist_enable=YES
userlist_deny=YES
userlist_file=/etc/vsftpd.user_list
secure_chroot_dir=/var/run/vsftpd/empty
pam_service_name=siroc
vsftpd_log_file=/var/log/vsftpd.log
dual_log_enable=YES
pasv_enable=YES
pasv_min_port=30000
pasv_max_port=30100
seccomp_sandbox=NO
`
	if pasv != "" {
		conf += "pasv_address=" + pasv + "\n"
	}
	return conf
}

// vsftpdPAM is /etc/pam.d/siroc. Ubuntu's vsftpd PAM stack ends with
// pam_shells.so, which rejects /usr/sbin/nologin and surfaces as a bad password.
func vsftpdPAM() string {
	return `# Siroc FTP. nologin is allowed so SSH-off accounts and extra FTP logins can authenticate.
auth	required	pam_listfile.so item=user sense=deny file=/etc/ftpusers onerr=succeed
@include common-account
@include common-session
@include common-auth
`
}
