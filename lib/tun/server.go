package tun

import (
    "net"
    "../tonnerre/golang-dns"
    "../ip"
)

type Conn struct {

    VAddr   *net.IPAddr
    PAddr   *net.UDPAddr

    Buffer  map[int][]byte
    TUN     *Tunnel
    DNS     *DNSUtils
}

type Server struct {

    /* Physical Address DNS Server Listening on */
    //LAddr   *UDPAddr

    /* Virtual Address in TUN Virtual Interface */
    VAddr   *net.IPAddr

    Routes_By_VAddr  map[string]*Conn
    Routes_By_PAddr  map[string]*Conn

    DNS     *DNSUtils
    TUN     *Tunnel
}

func NewServer(topDomain, laddr, vaddr, tunName string) (*Server, error){

    s := new(Server)
    /*
    s.LAddr, err := net.ResolveUDPAddr("udp", laddr)
    if err != nil {
        return nil, err
    }*/

    var err error
    s.VAddr, err = net.ResolveIPAddr("ip", vaddr)
    if err != nil {
        return nil, err
    }

    s.DNS, err = NewDNSServer(laddr, topDomain)
    if err != nil {
        return nil, err
    }

    s.TUN, err = NewTunnel(tunName)
    if err != nil {
        return nil, err
    }
    return s, nil
}

func (s *Server) NewConn(vaddr *net.IPAddr, paddr *net.UDPAddr) (*Conn){
    c := new(Conn)
    c.VAddr = vaddr
    c.PAddr = paddr
    c.TUN = s.TUN
    c.DNS = s.DNS
    c.Buffer = make(map[int][]byte)
    return c
}


func (c *Conn) Recv(t *TUNIPPacket){
    c.TUN.Save(c.Buffer, t)
}

func (s *Server) AcquireVAddr() *net.IPAddr{
    //addr := new(net.IPAddr)
    //*addr = *s.NextIPAddr

    // TODO
    Error.Println("Not Implemented")
    return nil
}

func (s *Server) DNSRecv(){

    b := make([]byte, DEF_BUF_SIZE)
    for {
        n, rpaddr, err := s.DNS.Conn.ReadFromUDP(b)
        if err != nil{
            Error.Println(err)
            continue
        }

        dnsPacket := new(dns.Msg)
        err = dnsPacket.Unpack(b[:n])
        //dnsPacket, err := s.DNS.Unpack(b[:n]) // TODO
        if err != nil {
            Error.Println(err)
            continue
        }
        tunPacket, err := s.DNS.Retrieve(dnsPacket) // TODO
        if err != nil {
            Error.Println(err)
            continue
        }

        switch tunPacket.GetCmd() {
        case TUN_CMD_CONNECT:

            rvaddr := s.AcquireVAddr()  //TODO

            // create new connection for the client
            conn := s.NewConn(rvaddr, rpaddr)
            s.Routes_By_VAddr[rvaddr.String()] = conn
            s.Routes_By_PAddr[rpaddr.String()] = conn

            t := new(TUNResponsePacket)
            t.Cmd = TUN_CMD_RESPONSE
            t.Server = s.VAddr    // server's virtual ip address
            t.Client = rvaddr     // client's virtual ip address

            msgs, err := s.DNS.Inject(t) // TODO
            if err != nil {
                Error.Println(err)
                continue
            }

            if len(msgs) != 1 {
                Error.Println("CONNECT: should be one DNS Packet\n")
                continue
            }

            binary, err := msgs[0].Pack()
            err = s.DNS.SendTo( conn.PAddr, binary)
            if err != nil {
                Error.Println(err)
                continue
            }

            Debug.Printf("Connected with %s\n", conn.PAddr.String())

        case TUN_CMD_DATA:

            conn, ok := s.Routes_By_PAddr[rpaddr.String()]
            if !ok {
                Debug.Println("Cannot find Connection for %s\n", rpaddr.String())
                continue
            }

            // cast packet to TUNIPPacket TODO: test if it works
            t, ok := tunPacket.(*TUNIPPacket)
            if !ok {
                Error.Printf("Unexpected cast fail from TUNPacket to TUNIPPacket\n")
                continue
            }else{
                conn.Recv(t)
            }

        case TUN_CMD_KILL:

            conn, ok := s.Routes_By_PAddr[rpaddr.String()]
            if !ok {
                Debug.Println("Cannot find Conn for %s\n", rpaddr.String())
                continue
            }

            delete(s.Routes_By_PAddr, conn.PAddr.String())
            delete(s.Routes_By_VAddr, conn.VAddr.String())
            Debug.Printf("Close Conn with %s\n", conn.VAddr.String())

        default:
            Error.Println("Invalid TUN Cmd")
        }
    }
}

func (s *Server) TUNRecv(){

    b := make([]byte, DEF_BUF_SIZE )
    for {

        n, err := s.TUN.Read(b)
        if err != nil {
            Error.Println(err)
            continue
        }

        ippkt := new(ip.IPPacket)
        err = ippkt.Unmarshal(b[:n])
        if err != nil {
            Error.Println(err)
            continue
        }
        Debug.Printf("TUN: IP Packet from %s to %s\n",
            ip.IPAddrInt2Str(ippkt.Header.Src),
            ip.IPAddrInt2Str(ippkt.Header.Dst))

        rvaddrStr := ip.IPAddrInt2Str(ippkt.Header.Dst)
        conn, ok := s.Routes_By_VAddr[rvaddrStr]
        if !ok {
            Debug.Printf("Connection to vip %s not found\n", rvaddrStr)
            continue
        }

        err = s.DNS.InjectAndSendTo(b[:n], conn.PAddr )
        if err != nil {
            Error.Println(err)
            continue
        }
    }
}
